package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"enterprise-pdf-ai/internal/repository"

	"go.uber.org/zap"
)

// ============ 混合检索服务 ============

// HybridSearchConfig 混合检索配置
type HybridSearchConfig struct {
	DenseWeight  float32 // Dense向量权重
	SparseWeight float32 // BM25权重
	TopK         int     // 返回数量
	RerankTopK   int     // Reranker后返回数量
}

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
}

// hybridSearchService 实现混合检索
type hybridSearchService struct {
	vectorStore  repository.NotebookVectorStore
	bm25Index    *BM25Index
	reranker    RerankerService
	embedder    interface{} // embeddings.Embedder
	config      *HybridSearchConfig
}

// NewHybridSearchService 创建混合检索服务
func NewHybridSearchService(
	vectorStore repository.NotebookVectorStore,
	bm25Index *BM25Index,
	reranker RerankerService,
	embedder interface{},
	config *HybridSearchConfig,
) HybridSearchService {
	if config == nil {
		config = &HybridSearchConfig{
			DenseWeight:  0.7,
			SparseWeight: 0.3,
			TopK:         20,
			RerankTopK:   5,
		}
	}
	return &hybridSearchService{
		vectorStore: vectorStore,
		bm25Index:   bm25Index,
		reranker:    reranker,
		embedder:    embedder,
		config:      config,
	}
}

// Search 执行混合检索
func (s *hybridSearchService) Search(ctx context.Context, query string, notebookID string, docIDs []string, topK int) ([]HybridResult, error) {
	if topK <= 0 {
		topK = s.config.TopK
	}
	rerankTopK := s.config.RerankTopK
	if rerankTopK > topK {
		rerankTopK = topK
	}

	var wg sync.WaitGroup
	var denseResults []HybridResult
	var sparseResults []HybridResult
	var denseErr, sparseErr error

	// 1. 并发执行 Dense 和 Sparse 检索
	wg.Add(2)

	go func() {
		defer wg.Done()
		denseResults, denseErr = s.searchDense(ctx, query, notebookID, docIDs, topK)
	}()

	go func() {
		defer wg.Done()
		sparseResults, sparseErr = s.searchSparse(ctx, query, notebookID, docIDs, topK)
	}()

	wg.Wait()

	if denseErr != nil && sparseErr != nil {
		return nil, fmt.Errorf("both search failed: dense=%v, sparse=%v", denseErr, sparseErr)
	}

	// 2. 合并结果
	merged := s.mergeResults(denseResults, sparseResults, topK)

	// 3. Rerank
	if s.reranker != nil && len(merged) > 0 {
		reranked, err := s.reranker.Rerank(ctx, query, merged, rerankTopK)
		if err != nil {
			zap.L().Warn("rerank failed, using merged results", zap.Error(err))
		} else {
			merged = reranked
		}
	}

	// 4. 更新排名
	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged, nil
}

// searchDense 执行向量检索
func (s *hybridSearchService) searchDense(ctx context.Context, query string, notebookID string, docIDs []string, topK int) ([]HybridResult, error) {
	if s.vectorStore == nil {
		return nil, fmt.Errorf("vector store not available")
	}

	// 获取 query 向量
	queryVector, err := s.embedQuery(ctx, query)
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
			ChunkID:    fmt.Sprintf("%d", chunk.ID),
			DocumentID: chunk.DocumentID,
			Content:    chunk.Content,
			Score:      scores[i],
			Metadata: map[string]interface{}{
				"page_number": chunk.PageNumber,
				"chunk_index": chunk.ChunkIndex,
			},
		})
	}

	return results, nil
}

// searchSparse 执行 BM25 检索
func (s *hybridSearchService) searchSparse(ctx context.Context, query string, notebookID string, docIDs []string, topK int) ([]HybridResult, error) {
	if s.bm25Index == nil {
		return nil, fmt.Errorf("bm25 index not available")
	}

	results, err := s.bm25Index.Search(query, topK, docIDs)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// mergeResults 合并 Dense 和 Sparse 结果
func (s *hybridSearchService) mergeResults(dense, sparse []HybridResult, topK int) []HybridResult {
	scoreMap := make(map[string]*HybridResult)

	// 归一化并加权 Dense 结果
	for _, r := range dense {
		normalized := 1.0 / (1.0 + float64(r.Score))
		result := r
		result.Score = float32(float64(s.config.DenseWeight) * normalized)
		scoreMap[r.ChunkID] = &result
	}

	// 归一化并加权 Sparse 结果
	for _, r := range sparse {
		normalized := 1.0 / (1.0 + float64(r.Score))
		if existing, ok := scoreMap[r.ChunkID]; ok {
			existing.Score += float32(float64(s.config.SparseWeight) * normalized)
		} else {
			result := r
			result.Score = float32(float64(s.config.SparseWeight) * normalized)
			scoreMap[r.ChunkID] = &result
		}
	}

	// 排序
	results := make([]HybridResult, 0, len(scoreMap))
	for _, r := range scoreMap {
		results = append(results, *r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// embedQuery 获取查询向量
func (s *hybridSearchService) embedQuery(ctx context.Context, query string) ([]float32, error) {
	// 使用 embedder 获取向量
	// 简化实现
	return make([]float32, 1536), nil
}

// ============ BM25 索引 ============

// BM25Index BM25 稀疏检索索引
type BM25Index struct {
	documents map[string][]string // chunkID -> tokens
	docCount  int
	avgDL     float64
	k1        float64
	b         float64
}

// BM25Result BM25 检索结果
type BM25Result struct {
	ChunkID   string
	DocumentID string
	Content   string
	Score     float32
}

// NewBM25Index 创建 BM25 索引
func NewBM25Index() *BM25Index {
	return &BM25Index{
		documents: make(map[string][]string),
		k1:        1.5,
		b:         0.75,
	}
}

// IndexDocument 添加文档到索引
func (idx *BM25Index) IndexDocument(chunkID, docID, content string) {
	tokens := tokenize(content)
	idx.documents[chunkID] = tokens
	idx.docCount++
}

// Search 搜索
func (idx *BM25Index) Search(query string, topK int, docIDs []string) ([]HybridResult, error) {
	queryTokens := tokenize(query)
	var results []HybridResult

	docScores := make(map[string]float64)
	docContents := make(map[string]string)
	docDocIDs := make(map[string]string)

	// 计算每个文档的 BM25 分数
	for chunkID, tokens := range idx.documents {
		// 过滤 docIDs
		if len(docIDs) > 0 {
			found := false
			for _, did := range docIDs {
				if strings.Contains(chunkID, did) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		score := idx.calculateBM25(queryTokens, tokens)
		docScores[chunkID] = score
		docContents[chunkID] = strings.Join(tokens, " ")
	}

	// 排序
	type scoredDoc struct {
		chunkID   string
		score     float64
		content   string
		documentID string
	}
	var scored []scoredDoc
	for chunkID, score := range docScores {
		scored = append(scored, scoredDoc{
			chunkID:   chunkID,
			score:     score,
			content:   docContents[chunkID],
			documentID: docDocIDs[chunkID],
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 取 TopK
	for i := 0; i < topK && i < len(scored); i++ {
		results = append(results, HybridResult{
			ChunkID:    scored[i].chunkID,
			DocumentID: scored[i].documentID,
			Content:    scored[i].content,
			Score:      float32(scored[i].score),
		})
	}

	return results, nil
}

// calculateBM25 计算 BM25 分数
func (idx *BM25Index) calculateBM25(queryTokens, docTokens []string) float64 {
	var score float64
	docLen := float64(len(docTokens))

	// 文档频率
	docFreq := make(map[string]int)
	for _, token := range docTokens {
		docFreq[token]++
	}

	// IDF
	idf := math.Log((float64(idx.docCount) - 0.5 + 0.5) / 0.5)

	for _, qToken := range queryTokens {
		if df, ok := docFreq[qToken]; ok {
			tf := float64(df)
			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1 - idx.b + idx.b*docLen/idx.avgDL)
			score += idf * numerator / denominator
		}
	}

	return score
}

// tokenize 简单分词
func tokenize(text string) []string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	// 简单分词
	var tokens []string
	var current strings.Builder
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			current.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			current.WriteRune(r + 32) // 转小写
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

// ============ Reranker 服务 ============

// RerankerService Reranker 接口
type RerankerService interface {
	Rerank(ctx context.Context, query string, candidates []HybridResult, topK int) ([]HybridResult, error)
}

// crossEncoderReranker 基于 Cross-Encoder 的 Reranker
type crossEncoderReranker struct {
	embedder interface{}
	modelName string
}

// NewRerankerService 创建 Reranker 服务
func NewRerankerService(embedder interface{}, modelName string) RerankerService {
	return &crossEncoderReranker{
		embedder: embedder,
		modelName: modelName,
	}
}

// Rerank 重排序
func (r *crossEncoderReranker) Rerank(ctx context.Context, query string, candidates []HybridResult, topK int) ([]HybridResult, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}

	// Cross-Encoder 重排序逻辑
	// 实际实现应该调用专门的 Reranker 模型 (如 BAAI/bge-reranker)
	// 这里简化实现

	type candidateWithScore struct {
		result HybridResult
		score  float32
	}

	scored := make([]candidateWithScore, len(candidates))
	for i, c := range candidates {
		// 简化的相似度计算
		score := r.simpleSimilarity(query, c.Content)
		scored[i] = candidateWithScore{result: c, score: score}
	}

	// 排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 取 TopK
	if topK > len(scored) {
		topK = len(scored)
	}
	results := make([]HybridResult, topK)
	for i := 0; i < topK; i++ {
		results[i] = scored[i].result
		results[i].Score = scored[i].score
	}

	return results, nil
}

// simpleSimilarity 简化相似度
func (r *crossEncoderReranker) simpleSimilarity(query, content string) float32 {
	queryTokens := tokenize(strings.ToLower(query))
	contentTokens := tokenize(strings.ToLower(content))

	if len(queryTokens) == 0 || len(contentTokens) == 0 {
		return 0
	}

	match := 0
	for _, qt := range queryTokens {
		for _, ct := range contentTokens {
			if qt == ct {
				match++
				break
			}
		}
	}

	return float32(match) / float32(len(queryTokens))
}
