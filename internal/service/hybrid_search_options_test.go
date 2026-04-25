package service

import (
	"context"
	"testing"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/repository"
)

type fakeHybridEmbedder struct {
	queries []string
}

func (f *fakeHybridEmbedder) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	f.queries = append(f.queries, text)
	if text == "revenue" {
		return []float32{2}, nil
	}
	return []float32{1}, nil
}

func (f *fakeHybridEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{1}
	}
	return vectors, nil
}

type fakeNotebookVectorStore struct{}

func (f *fakeNotebookVectorStore) InsertChunks(context.Context, []repository.NotebookChunk, [][]float32) error {
	return nil
}

func (f *fakeNotebookVectorStore) Search(_ context.Context, queryVector []float32, _ int, _ string, _ []string) ([]repository.NotebookChunk, []float32, error) {
	if len(queryVector) > 0 && queryVector[0] == 2 {
		return []repository.NotebookChunk{{ID: 2, DocumentID: "doc", Content: "retry result", ChunkIndex: 1}}, []float32{0.92}, nil
	}
	return []repository.NotebookChunk{{ID: 1, DocumentID: "doc", Content: "low confidence", ChunkIndex: 0}}, []float32{0.05}, nil
}

func (f *fakeNotebookVectorStore) DeleteByDocument(context.Context, string) error { return nil }
func (f *fakeNotebookVectorStore) DeleteByNotebook(context.Context, string) error { return nil }
func (f *fakeNotebookVectorStore) GetAllChunks(context.Context) ([]repository.NotebookChunk, error) {
	return nil, nil
}

type duplicateZeroIDVectorStore struct{}

func (f *duplicateZeroIDVectorStore) InsertChunks(context.Context, []repository.NotebookChunk, [][]float32) error {
	return nil
}

func (f *duplicateZeroIDVectorStore) Search(context.Context, []float32, int, string, []string) ([]repository.NotebookChunk, []float32, error) {
	return []repository.NotebookChunk{
		{DocumentID: "doc-a", Content: "first visual chart evidence", ChunkIndex: 0},
		{DocumentID: "doc-a", Content: "second table evidence", ChunkIndex: 1},
	}, []float32{0.91, 0.89}, nil
}

func (f *duplicateZeroIDVectorStore) DeleteByDocument(context.Context, string) error { return nil }
func (f *duplicateZeroIDVectorStore) DeleteByNotebook(context.Context, string) error { return nil }
func (f *duplicateZeroIDVectorStore) GetAllChunks(context.Context) ([]repository.NotebookChunk, error) {
	return nil, nil
}

type allChunksVectorStore struct {
	chunks []repository.NotebookChunk
}

func (f *allChunksVectorStore) InsertChunks(context.Context, []repository.NotebookChunk, [][]float32) error {
	return nil
}

func (f *allChunksVectorStore) Search(context.Context, []float32, int, string, []string) ([]repository.NotebookChunk, []float32, error) {
	return nil, nil, nil
}

func (f *allChunksVectorStore) DeleteByDocument(context.Context, string) error { return nil }
func (f *allChunksVectorStore) DeleteByNotebook(context.Context, string) error { return nil }
func (f *allChunksVectorStore) GetAllChunks(context.Context) ([]repository.NotebookChunk, error) {
	return f.chunks, nil
}

type recordingIntentRewrite struct {
	userID    string
	sessionID string
	query     string
}

func (r *recordingIntentRewrite) Rewrite(_ context.Context, userID, sessionID, query string) (*RewriteResult, error) {
	r.userID = userID
	r.sessionID = sessionID
	r.query = query
	return &RewriteResult{OriginalQuery: query, RewrittenQuery: query}, nil
}

func (r *recordingIntentRewrite) IdentifyIntent(string) QueryIntent {
	return IntentUnknown
}

func TestHybridSearchRetriesLowConfidenceDenseResults(t *testing.T) {
	embedder := &fakeHybridEmbedder{}
	svc := NewHybridSearchService(
		&fakeNotebookVectorStore{},
		nil,
		nil,
		NewFailoverStrategy(&configs.HybridSearchConfig{MinConfidence: 0.3, MaxRetries: 1}, NewTokenizer()),
		embedder,
		nil,
		&configs.HybridSearchConfig{Enabled: true, TopK: 1, RerankTopK: 1, MinConfidence: 0.3, MaxRetries: 1},
	)

	results, err := svc.Search(context.Background(), "what is revenue", "nb", []string{"doc"}, 1)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Content != "retry result" {
		t.Fatalf("expected retry result, got %q", results[0].Content)
	}
	if len(embedder.queries) != 2 {
		t.Fatalf("expected original search plus retry, got queries %#v", embedder.queries)
	}
}

func TestHybridSearchWithOptionsPassesUserAndSessionToIntentRewrite(t *testing.T) {
	rewrite := &recordingIntentRewrite{}
	svc := NewHybridSearchService(
		&fakeNotebookVectorStore{},
		nil,
		nil,
		nil,
		&fakeHybridEmbedder{},
		rewrite,
		&configs.HybridSearchConfig{Enabled: true, TopK: 1, RerankTopK: 1},
	)

	_, err := svc.SearchWithOptions(context.Background(), HybridSearchOptions{
		Query:       "follow up",
		UserID:      "user-1",
		SessionID:   "session-1",
		NotebookID:  "nb",
		DocumentIDs: []string{"doc"},
		TopK:        1,
	})
	if err != nil {
		t.Fatalf("SearchWithOptions returned error: %v", err)
	}

	if rewrite.userID != "user-1" || rewrite.sessionID != "session-1" || rewrite.query != "follow up" {
		t.Fatalf("rewrite context mismatch: user=%q session=%q query=%q", rewrite.userID, rewrite.sessionID, rewrite.query)
	}
}

func TestDenseSearchUsesStableChunkIDsWhenMilvusPrimaryIDMissing(t *testing.T) {
	svc := NewHybridSearchService(
		&duplicateZeroIDVectorStore{},
		nil,
		nil,
		nil,
		&fakeHybridEmbedder{},
		nil,
		&configs.HybridSearchConfig{Enabled: true, TopK: 2, RerankTopK: 2},
	)

	results, err := svc.Search(context.Background(), "chart table", "nb", []string{"doc-a"}, 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected both dense results to survive, got %d: %#v", len(results), results)
	}
	if results[0].ChunkID != "nb_doc-a_0" || results[1].ChunkID != "nb_doc-a_1" {
		t.Fatalf("expected stable chunk ids, got %q and %q", results[0].ChunkID, results[1].ChunkID)
	}
}

func TestFailoverRetriesSinglePathRRFButKeepsDualPathRRF(t *testing.T) {
	failover := NewFailoverStrategy(&configs.HybridSearchConfig{
		RRFK:          60,
		MinConfidence: 0.3,
		MaxRetries:    1,
	}, NewTokenizer())

	singlePathResults := []HybridResult{
		{
			ChunkID: "chunk-1",
			Score:   1.0 / 61.0,
			Metadata: map[string]interface{}{
				"score_type": "rrf",
				"rrf_hits":   1,
			},
		},
	}
	if !failover.ShouldRetry(singlePathResults, 0) {
		t.Fatalf("expected single-path RRF top result to trigger failover retry")
	}

	dualPathResults := []HybridResult{
		{
			ChunkID: "chunk-2",
			Score:   2.0 / 61.0,
			Metadata: map[string]interface{}{
				"score_type": "rrf",
				"rrf_hits":   2,
			},
		},
	}
	if failover.ShouldRetry(dualPathResults, 0) {
		t.Fatalf("expected dual-path RRF top result not to trigger failover retry")
	}
}

func TestPrioritizeEvidenceTypesBoostsTableForTableQueries(t *testing.T) {
	results := []HybridResult{
		{ChunkID: "text", Score: 0.90, Metadata: map[string]interface{}{"chunk_type": "text"}},
		{ChunkID: "table", Score: 0.89, Metadata: map[string]interface{}{"chunk_type": "table"}},
	}

	prioritized := prioritizeEvidenceTypes("quarterly revenue table", results)
	if prioritized[0].ChunkID != "table" {
		t.Fatalf("expected table evidence first, got %#v", prioritized)
	}
}

func TestPrioritizeEvidenceTypesLeavesPlainQueriesUnchanged(t *testing.T) {
	results := []HybridResult{
		{ChunkID: "text", Score: 0.90, Metadata: map[string]interface{}{"chunk_type": "text"}},
		{ChunkID: "table", Score: 0.89, Metadata: map[string]interface{}{"chunk_type": "table"}},
	}

	prioritized := prioritizeEvidenceTypes("main business segments", results)
	if prioritized[0].ChunkID != "text" {
		t.Fatalf("expected plain query ordering unchanged, got %#v", prioritized)
	}
}

func TestBM25RefreshPreservesChunkMetadataForSparseResults(t *testing.T) {
	idx := NewBM25Index(NewTokenizer())
	_, err := idx.RefreshFromStore(context.Background(), &allChunksVectorStore{chunks: []repository.NotebookChunk{
		{
			DocumentID:  "doc-1",
			PageNumber:  2,
			ChunkIndex:  7,
			Content:     "quarterly revenue table cloud margin uniquealpha",
			ChunkType:   "table",
			ChunkRole:   "parent",
			ParentID:    "parent-1",
			SectionPath: `["Financial Tables","Quarterly Revenue"]`,
			BBox:        `[10,20,200,240]`,
		},
		{
			DocumentID: "doc-2",
			Content:    "ordinary narrative unrelated content uniquebeta",
		},
		{
			DocumentID: "doc-3",
			Content:    "another ordinary narrative uniquegamma",
		},
	}})
	if err != nil {
		t.Fatalf("RefreshFromStore returned error: %v", err)
	}

	results, err := idx.Search("uniquealpha", 1, []string{"doc-1"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one sparse result, got %d", len(results))
	}
	if got := results[0].Metadata["chunk_type"]; got != "table" {
		t.Fatalf("expected chunk_type table metadata, got %#v", got)
	}
	if got := results[0].Metadata["page_number"]; got != int64(2) {
		t.Fatalf("expected page_number metadata, got %#v", got)
	}
}
