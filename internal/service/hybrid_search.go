package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/repository"

	"github.com/tmc/langchaingo/embeddings"
	"go.uber.org/zap"
)

// ============ 混合检索服务 (Phase 2: 生产级 Hybrid RAG) ============

// HybridResult 混合检索结果
type HybridResult struct {
	ChunkID    string
	DocumentID string
	Content    string
	Score      float32
	Rank       int
	Metadata   map[string]interface{}
}

// HybridSearchService 混合检索接口
type HybridSearchService interface {
	Search(ctx context.Context, query string, notebookID string, docIDs []string, topK int) ([]HybridResult, error)
	SearchWithOptions(ctx context.Context, opts HybridSearchOptions) ([]HybridResult, error)
}

// HybridSearchOptions carries retrieval scope and conversational context.
type HybridSearchOptions struct {
	Query       string
	UserID      string
	SessionID   string
	NotebookID  string
	DocumentIDs []string
	TopK        int
}

// hybridSearchService 实现混合检索
type hybridSearchService struct {
	vectorStore   repository.NotebookVectorStore
	bm25Index     *BM25Index
	reranker      RerankerService
	failover      *FailoverStrategy
	embedder      embeddings.Embedder
	config        *configs.HybridSearchConfig
	intentRewrite IntentRewriteService
}

// NewHybridSearchService 创建混合检索服务
func NewHybridSearchService(
	vectorStore repository.NotebookVectorStore,
	bm25Index *BM25Index,
	reranker RerankerService,
	failover *FailoverStrategy,
	embedder embeddings.Embedder,
	intentRewrite IntentRewriteService,
	config *configs.HybridSearchConfig,
) HybridSearchService {
	if config == nil {
		config = &configs.HybridSearchConfig{
			Enabled:       true,
			RRFK:          60,
			TopK:          20,
			RerankTopK:    5,
			MinConfidence: 0.3,
			MaxRetries:    1,
		}
	}
	return &hybridSearchService{
		vectorStore:   vectorStore,
		bm25Index:     bm25Index,
		reranker:      reranker,
		failover:      failover,
		embedder:      embedder,
		intentRewrite: intentRewrite,
		config:        config,
	}
}

// Search 执行混合检索（完整管线：意图重写 → 双路召回 → RRF 融合 → Rerank → Failover）
func (s *hybridSearchService) Search(ctx context.Context, query string, notebookID string, docIDs []string, topK int) ([]HybridResult, error) {
	return s.SearchWithOptions(ctx, HybridSearchOptions{
		Query:       query,
		NotebookID:  notebookID,
		DocumentIDs: docIDs,
		TopK:        topK,
	})
}

// SearchWithOptions 执行混合检索，并允许调用方传入用户/会话上下文供意图重写使用。
func (s *hybridSearchService) SearchWithOptions(ctx context.Context, opts HybridSearchOptions) ([]HybridResult, error) {
	query := opts.Query
	topK := opts.TopK
	if topK <= 0 {
		topK = s.config.TopK
	}
	rerankTopK := s.config.RerankTopK
	if rerankTopK <= 0 || rerankTopK > topK {
		rerankTopK = topK
	}

	// Step 0: 意图重写（如果启用）
	if s.intentRewrite != nil {
		rewriteResult, err := s.intentRewrite.Rewrite(ctx, opts.UserID, opts.SessionID, query)
		if err != nil {
			zap.L().Warn("intent rewrite failed, using original query", zap.Error(err))
		} else if rewriteResult != nil && rewriteResult.RewrittenQuery != "" {
			zap.L().Debug("query rewritten",
				zap.String("original", query),
				zap.String("rewritten", rewriteResult.RewrittenQuery),
				zap.String("intent", string(rewriteResult.Intent)),
			)
			query = rewriteResult.RewrittenQuery
		}
	}

	merged, err := s.searchOnce(ctx, query, opts.NotebookID, opts.DocumentIDs, topK, rerankTopK)
	if err != nil {
		return nil, err
	}

	// Step 4: Failover 检查与真实二次检索
	if s.failover != nil && s.failover.ShouldRetry(merged, 0) {
		reason := "no results returned"
		if len(merged) > 0 {
			reason = fmt.Sprintf("low confidence score: %.4f < %.4f", merged[0].Score, s.failover.minConfidenceScore)
		}
		retryResults, retryErr := s.failover.ExecuteRetry(ctx, query, func(ctx context.Context, retryQuery string) ([]HybridResult, error) {
			return s.searchOnce(ctx, retryQuery, opts.NotebookID, opts.DocumentIDs, topK, rerankTopK)
		})
		if retryErr != nil {
			zap.L().Warn("failover retry failed, keeping original results",
				zap.String("reason", reason),
				zap.Error(retryErr),
			)
		} else if len(retryResults) > 0 {
			zap.L().Info("failover retry returned replacement results",
				zap.String("reason", reason),
				zap.Int("result_count", len(retryResults)),
			)
			merged = retryResults
		}
	}

	// Step 5: 更新排名
	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged, nil
}

func (s *hybridSearchService) searchOnce(ctx context.Context, query string, notebookID string, docIDs []string, topK int, rerankTopK int) ([]HybridResult, error) {
	// Step 0.5: 如果 BM25 索引为空，走纯 dense 快速路径
	// 跳过 sparse 检索和 RRF 融合，确保与 Phase 1 纯 dense 行为兼容
	if s.bm25Index == nil || s.bm25Index.GetDocCount() == 0 {
		denseResults, err := s.searchDense(ctx, query, notebookID, docIDs, topK)
		if err != nil {
			return nil, fmt.Errorf("dense search: %w", err)
		}
		// Rerank 仍可受益
		if s.reranker != nil && len(denseResults) > 0 {
			reranked, rerankErr := s.reranker.Rerank(ctx, query, denseResults, rerankTopK)
			if rerankErr != nil {
				zap.L().Warn("rerank failed, using dense results", zap.Error(rerankErr))
			} else {
				denseResults = reranked
			}
		}
		for i := range denseResults {
			denseResults[i].Rank = i + 1
		}
		return denseResults, nil
	}

	// Step 1: 并发执行 Dense 和 Sparse 检索
	var wg sync.WaitGroup
	var denseResults []HybridResult
	var sparseResults []HybridResult
	var denseErr, sparseErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		denseResults, denseErr = s.searchDense(ctx, query, notebookID, docIDs, topK)
	}()
	go func() {
		defer wg.Done()
		sparseResults, sparseErr = s.searchSparse(ctx, query, docIDs, topK)
	}()
	wg.Wait()

	if denseErr != nil && sparseErr != nil {
		return nil, fmt.Errorf("both search paths failed: dense=%v, sparse=%v", denseErr, sparseErr)
	}

	// 单路降级
	if denseErr != nil {
		zap.L().Warn("dense search failed, using sparse only", zap.Error(denseErr))
		// 直接使用 sparse 结果
		if len(sparseResults) > topK {
			sparseResults = sparseResults[:topK]
		}
		for i := range sparseResults {
			sparseResults[i].Rank = i + 1
		}
		return sparseResults, nil
	}
	if sparseErr != nil {
		zap.L().Warn("sparse search failed, using dense only", zap.Error(sparseErr))
		if len(denseResults) > topK {
			denseResults = denseResults[:topK]
		}
		for i := range denseResults {
			denseResults[i].Rank = i + 1
		}
		return denseResults, nil
	}

	// Step 2: RRF 融合（仅当两路都有结果时才融合；BM25索引为空时sparse为空，直接用dense结果）
	var merged []HybridResult
	if len(sparseResults) == 0 {
		// 无sparse结果，直接使用dense结果（保留原始cosine分数）
		merged = denseResults
		if len(merged) > topK {
			merged = merged[:topK]
		}
	} else if len(denseResults) == 0 {
		// 无dense结果，直接使用sparse结果
		merged = sparseResults
		if len(merged) > topK {
			merged = merged[:topK]
		}
	} else {
		// 两路都有结果，执行RRF融合
		merged = s.mergeResultsRRF(denseResults, sparseResults, topK)
	}

	// Step 3: Rerank（如果配置了 Reranker）
	if s.reranker != nil && len(merged) > 0 {
		reranked, err := s.reranker.Rerank(ctx, query, merged, rerankTopK)
		if err != nil {
			zap.L().Warn("rerank failed, using RRF merged results", zap.Error(err))
		} else {
			merged = reranked
		}
	}

	merged = prioritizeEvidenceTypes(query, merged)

	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged, nil
}

func prioritizeEvidenceTypes(query string, results []HybridResult) []HybridResult {
	if len(results) < 2 {
		return results
	}
	query = strings.ToLower(query)
	wantsTable := hasAny(query, "table", "quarterly", "margin", "product line", "breakdown", "expenditure", "表格", "明细")
	wantsVisual := hasAny(query, "chart", "graph", "pie chart", "bar chart", "org chart", "image", "shown", "figure", "diagram", "trend", "plot", "图", "图表", "图片", "趋势", "曲线", "柱状", "饼图", "组织结构")
	if !wantsTable && !wantsVisual {
		return results
	}

	prioritized := append([]HybridResult(nil), results...)
	for i := range prioritized {
		chunkType := strings.ToLower(metadataStringAny(prioritized[i].Metadata, "chunk_type"))
		visualType := strings.ToLower(metadataStringAny(prioritized[i].Metadata, "visual_type"))
		content := strings.ToLower(prioritized[i].Content)
		if wantsTable && chunkType == "table" {
			prioritized[i].Score += 0.05
			continue
		}
		if wantsVisual && (chunkType == "image" || chunkType == "caption" || visualType != "" || strings.Contains(content, "[visual:")) {
			prioritized[i].Score += 0.08
			if visualType == "chart" && hasAny(query, "chart", "graph", "trend", "plot", "图表", "趋势", "曲线", "柱状", "饼图") {
				prioritized[i].Score += 0.04
			}
		}
	}
	sort.SliceStable(prioritized, func(i, j int) bool {
		return prioritized[i].Score > prioritized[j].Score
	})
	return prioritized
}

// searchDense 执行向量检索
func (s *hybridSearchService) searchDense(ctx context.Context, query string, notebookID string, docIDs []string, topK int) ([]HybridResult, error) {
	if s.vectorStore == nil {
		return nil, fmt.Errorf("vector store not available")
	}
	if s.embedder == nil {
		return nil, fmt.Errorf("embedder not available")
	}

	// 获取 query 向量
	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	chunks, scores, err := s.vectorStore.Search(ctx, queryVector, topK, notebookID, docIDs)
	if err != nil {
		return nil, err
	}

	results := make([]HybridResult, 0, len(chunks))
	for i, chunk := range chunks {
		results = append(results, HybridResult{
			ChunkID:    stableNotebookChunkID(chunk),
			DocumentID: chunk.DocumentID,
			Content:    chunk.Content,
			Score:      scores[i],
			Metadata: map[string]interface{}{
				"page_number":  chunk.PageNumber,
				"chunk_index":  chunk.ChunkIndex,
				"chunk_type":   chunk.ChunkType,
				"chunk_role":   chunk.ChunkRole,
				"parent_id":    chunk.ParentID,
				"section_path": chunk.SectionPath,
				"bbox":         chunk.BBox,
				"score_type":   "dense",
			},
		})
	}

	return results, nil
}

func stableNotebookChunkID(chunk repository.NotebookChunk) string {
	if chunk.ID > 0 {
		return fmt.Sprintf("%d", chunk.ID)
	}
	return fmt.Sprintf("nb_%s_%d", chunk.DocumentID, chunk.ChunkIndex)
}

// searchSparse 执行 BM25 检索
func (s *hybridSearchService) searchSparse(ctx context.Context, query string, docIDs []string, topK int) ([]HybridResult, error) {
	if s.bm25Index == nil {
		return nil, fmt.Errorf("bm25 index not available")
	}

	results, err := s.bm25Index.Search(query, topK, docIDs)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// mergeResultsRRF 使用 Reciprocal Rank Fusion 合并 Dense 和 Sparse 结果
// RRF_score(d) = Σ 1/(k + rank_i)，k 通常取 60
func (s *hybridSearchService) mergeResultsRRF(dense, sparse []HybridResult, topK int) []HybridResult {
	rrfK := s.config.RRFK
	if rrfK <= 0 {
		rrfK = 60
	}

	scoreMap := make(map[string]float32)       // chunkID → RRF score
	resultMap := make(map[string]HybridResult) // chunkID → result
	contributionMap := make(map[string]map[string]int)

	// Dense 路贡献
	for i, r := range dense {
		if _, exists := scoreMap[r.ChunkID]; !exists {
			scoreMap[r.ChunkID] = 0
			resultMap[r.ChunkID] = r
		}
		scoreMap[r.ChunkID] += 1.0 / float32(rrfK+i+1)
		rankMap := ensureRRFContribution(contributionMap, r.ChunkID)
		rankMap["dense_rank"] = i + 1
	}

	// Sparse 路贡献
	for i, r := range sparse {
		if _, exists := scoreMap[r.ChunkID]; !exists {
			scoreMap[r.ChunkID] = 0
			resultMap[r.ChunkID] = r
		}
		scoreMap[r.ChunkID] += 1.0 / float32(rrfK+i+1)
		rankMap := ensureRRFContribution(contributionMap, r.ChunkID)
		rankMap["sparse_rank"] = i + 1
	}

	// 按 RRF 分数排序，取 TopK
	candidates := make([]HybridResult, 0, len(scoreMap))
	for id, score := range scoreMap {
		r := resultMap[id]
		r.Score = score
		if r.Metadata == nil {
			r.Metadata = make(map[string]interface{})
		}
		r.Metadata["score_type"] = "rrf"
		r.Metadata["rrf_hits"] = len(contributionMap[id])
		for key, value := range contributionMap[id] {
			r.Metadata[key] = value
		}
		candidates = append(candidates, r)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	return candidates
}

func ensureRRFContribution(contributionMap map[string]map[string]int, chunkID string) map[string]int {
	rankMap, ok := contributionMap[chunkID]
	if !ok {
		rankMap = make(map[string]int, 2)
		contributionMap[chunkID] = rankMap
	}
	return rankMap
}

// ============ BM25 索引（Phase 2 修复版） ============

// BM25Index BM25 稀疏检索索引
type BM25Index struct {
	mu          sync.RWMutex
	documents   map[string][]string // chunkID → tokens
	docIDs      map[string]string   // chunkID → documentID
	contents    map[string]string   // chunkID → original content
	metadata    map[string]map[string]interface{}
	docFreq     map[string]int // term → 包含该 term 的文档数
	docCount    int            // 文档总数 N
	totalDocLen int            // 所有文档 token 数之和
	avgDL       float64        // 平均文档长度
	k1          float64        // BM25 参数，默认 1.5
	b           float64        // BM25 参数，默认 0.75
	tokenizer   *Tokenizer     // 统一分词器
}

// NewBM25Index 创建 BM25 索引
func NewBM25Index(tokenizer *Tokenizer) *BM25Index {
	return &BM25Index{
		documents: make(map[string][]string),
		docIDs:    make(map[string]string),
		contents:  make(map[string]string),
		metadata:  make(map[string]map[string]interface{}),
		docFreq:   make(map[string]int),
		k1:        1.5,
		b:         0.75,
		tokenizer: tokenizer,
	}
}

// IndexDocument 添加文档到索引
func (idx *BM25Index) IndexDocument(chunkID, docID, content string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.indexDocumentLocked(chunkID, docID, content, nil)
}

func (idx *BM25Index) IndexDocumentWithMetadata(chunkID, docID, content string, metadata map[string]interface{}) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.indexDocumentLocked(chunkID, docID, content, metadata)
}

func (idx *BM25Index) indexDocumentLocked(chunkID, docID, content string, metadata map[string]interface{}) {
	// 如果已存在，先移除旧的
	if _, exists := idx.documents[chunkID]; exists {
		idx.removeDocumentLocked(chunkID)
	}

	tokens := idx.tokenizer.Tokenize(content)
	if len(tokens) == 0 {
		return
	}

	idx.documents[chunkID] = tokens
	idx.docIDs[chunkID] = docID
	idx.contents[chunkID] = content
	idx.metadata[chunkID] = metadata
	idx.docCount++
	idx.totalDocLen += len(tokens)
	idx.avgDL = float64(idx.totalDocLen) / float64(idx.docCount)

	// 更新 DF：统计该文档中出现的唯一 term
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if _, ok := seen[token]; !ok {
			seen[token] = struct{}{}
			idx.docFreq[token]++
		}
	}
}

// RemoveByDocument 删除指定文档的所有 chunks
func (idx *BM25Index) RemoveByDocument(docID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for chunkID, dID := range idx.docIDs {
		if dID == docID {
			idx.removeDocumentLocked(chunkID)
		}
	}
}

// RemoveByChunkID 删除指定 chunk
func (idx *BM25Index) RemoveByChunkID(chunkID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeDocumentLocked(chunkID)
}

// removeDocumentLocked 内部删除方法（调用者需持有写锁）
func (idx *BM25Index) removeDocumentLocked(chunkID string) {
	tokens, exists := idx.documents[chunkID]
	if !exists {
		return
	}

	// 更新 DF
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if _, ok := seen[token]; !ok {
			seen[token] = struct{}{}
			idx.docFreq[token]--
			if idx.docFreq[token] <= 0 {
				delete(idx.docFreq, token)
			}
		}
	}

	idx.docCount--
	idx.totalDocLen -= len(tokens)
	delete(idx.documents, chunkID)
	delete(idx.docIDs, chunkID)
	delete(idx.contents, chunkID)
	delete(idx.metadata, chunkID)

	if idx.docCount > 0 {
		idx.avgDL = float64(idx.totalDocLen) / float64(idx.docCount)
	} else {
		idx.avgDL = 0
	}
}

// Search 搜索
func (idx *BM25Index) Search(query string, topK int, docIDs []string) ([]HybridResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.docCount == 0 {
		return nil, nil
	}

	queryTokens := idx.tokenizer.Tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	// 构建 docIDs 白名单
	docIDSet := make(map[string]struct{}, len(docIDs))
	for _, id := range docIDs {
		docIDSet[id] = struct{}{}
	}

	type scoredDoc struct {
		chunkID    string
		documentID string
		content    string
		score      float64
	}

	var scored []scoredDoc

	for chunkID, tokens := range idx.documents {
		// 精确匹配 docIDs 白名单
		if len(docIDSet) > 0 {
			docID := idx.docIDs[chunkID]
			if _, ok := docIDSet[docID]; !ok {
				continue
			}
		}

		score := idx.calculateBM25(queryTokens, tokens)
		if score > 0 {
			scored = append(scored, scoredDoc{
				chunkID:    chunkID,
				documentID: idx.docIDs[chunkID],
				content:    idx.contents[chunkID],
				score:      score,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 取 TopK
	results := make([]HybridResult, 0, topK)
	for i := 0; i < topK && i < len(scored); i++ {
		results = append(results, HybridResult{
			ChunkID:    scored[i].chunkID,
			DocumentID: scored[i].documentID,
			Content:    scored[i].content,
			Score:      float32(scored[i].score),
			Metadata:   idx.sparseResultMetadata(scored[i].chunkID),
		})
	}

	return results, nil
}

func (idx *BM25Index) sparseResultMetadata(chunkID string) map[string]interface{} {
	out := make(map[string]interface{})
	for key, value := range idx.metadata[chunkID] {
		out[key] = value
	}
	out["score_type"] = "sparse"
	return out
}

// calculateBM25 计算 BM25 分数（修复版：每个 query term 独立 IDF）
func (idx *BM25Index) calculateBM25(queryTokens, docTokens []string) float64 {
	if idx.avgDL == 0 {
		return 0
	}

	var score float64
	docLen := float64(len(docTokens))

	// 文档内 term 频率
	docFreq := make(map[string]int, len(docTokens))
	for _, token := range docTokens {
		docFreq[token]++
	}

	for _, qToken := range queryTokens {
		tf, ok := docFreq[qToken]
		if !ok {
			continue
		}

		// 正确的 IDF：log((N - df + 0.5) / (df + 0.5))
		df := idx.docFreq[qToken] // 全局 DF
		idf := math.Log((float64(idx.docCount) - float64(df) + 0.5) / (float64(df) + 0.5))

		// 如果 IDF <= 0，跳过（该 term 在超过半数文档中出现，区分度低）
		if idf <= 0 {
			continue
		}

		tfFloat := float64(tf)
		numerator := tfFloat * (idx.k1 + 1)
		denominator := tfFloat + idx.k1*(1-idx.b+idx.b*docLen/idx.avgDL)
		score += idf * numerator / denominator
	}

	return score
}

// GetDocCount 获取索引文档数（用于调试）
func (idx *BM25Index) GetDocCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.docCount
}

// RefreshFromStore 从 NotebookVectorStore 增量刷新 BM25 索引
// 解决 API 进程和 Worker 进程是独立进程、BM25Index 实例不共享的问题
func (idx *BM25Index) RefreshFromStore(ctx context.Context, store repository.NotebookVectorStore) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("vector store is nil")
	}

	chunks, err := store.GetAllChunks(ctx)
	if err != nil {
		return 0, fmt.Errorf("get all chunks from store: %w", err)
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 构建当前 Milvus 中的 chunkID 集合
	storeChunkIDs := make(map[string]repository.NotebookChunk, len(chunks))
	for _, chunk := range chunks {
		chunkID := fmt.Sprintf("nb_%s_%d", chunk.DocumentID, chunk.ChunkIndex)
		storeChunkIDs[chunkID] = chunk
	}

	// 找出需要删除的旧 chunks（在 BM25 中但不在 Milvus 中）
	var toDelete []string
	for chunkID := range idx.documents {
		if _, exists := storeChunkIDs[chunkID]; !exists {
			toDelete = append(toDelete, chunkID)
		}
	}

	// 删除旧 chunks
	for _, chunkID := range toDelete {
		idx.removeDocumentLocked(chunkID)
	}

	// 添加新/更新的 chunks
	added := 0
	for chunkID, chunk := range storeChunkIDs {
		if _, exists := idx.documents[chunkID]; !exists {
			idx.indexDocumentLocked(chunkID, chunk.DocumentID, chunk.Content, notebookChunkMetadata(chunk))
			added++
		}
	}

	return added, nil
}

func NotebookChunkMetadata(chunk repository.NotebookChunk) map[string]interface{} {
	metadata := map[string]interface{}{
		"page_number":  chunk.PageNumber,
		"chunk_index":  chunk.ChunkIndex,
		"chunk_type":   chunk.ChunkType,
		"chunk_role":   chunk.ChunkRole,
		"parent_id":    chunk.ParentID,
		"section_path": chunk.SectionPath,
		"bbox":         chunk.BBox,
	}
	if path := extractVisualPathMarker(chunk.Content); path != "" {
		metadata["visual_path"] = path
	}
	if visualType := extractVisualTypeMarker(chunk.Content); visualType != "" {
		metadata["visual_type"] = visualType
	}
	return metadata
}

func notebookChunkMetadata(chunk repository.NotebookChunk) map[string]interface{} {
	return NotebookChunkMetadata(chunk)
}

// StartRefreshLoop 启动后台定时刷新 BM25 索引的 goroutine
// 每隔 interval 从 NotebookVectorStore 增量同步数据
func (idx *BM25Index) StartRefreshLoop(ctx context.Context, store repository.NotebookVectorStore, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				zap.L().Info("BM25 index refresh loop stopped")
				return
			case <-ticker.C:
				added, err := idx.RefreshFromStore(ctx, store)
				if err != nil {
					zap.L().Warn("BM25 index refresh failed", zap.Error(err))
				} else if added > 0 {
					zap.L().Info("BM25 index refreshed",
						zap.Int("added", added),
						zap.Int("total", idx.GetDocCount()),
					)
				}
			}
		}
	}()
	zap.L().Info("BM25 index refresh loop started", zap.Duration("interval", interval))
}

// ============ 兼容旧代码：保留 tokenize 函数签名（已废弃） ============

// tokenize 简单分词（已废弃，保留用于其他模块的旧引用，将在 Task 7/8 中替换）
// Deprecated: 使用 Tokenizer.Tokenize() 替代
func tokenize(text string) []string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	var tokens []string
	var current strings.Builder
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			current.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			current.WriteRune(r + 32)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}
