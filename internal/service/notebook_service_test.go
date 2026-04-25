package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/models"
	"NotebookAI/internal/parser"
	"NotebookAI/internal/repository"
)

// ErrNotFound is a mock error for testing
var ErrNotFound = errors.New("not found")

// MockNotebookRepository implements NotebookRepository for testing
type MockNotebookRepository struct {
	notebooks map[string]*models.Notebook
	guides    map[string]*models.DocumentGuide
	docs      map[string]*models.Document
	mu        sync.RWMutex
}

func NewMockNotebookRepository() *MockNotebookRepository {
	return &MockNotebookRepository{
		notebooks: make(map[string]*models.Notebook),
		guides:    make(map[string]*models.DocumentGuide),
		docs:      make(map[string]*models.Document),
	}
}

func (r *MockNotebookRepository) Create(ctx context.Context, notebook *models.Notebook) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notebooks[notebook.ID] = notebook
	return nil
}

func (r *MockNotebookRepository) GetByID(ctx context.Context, userID, notebookID string) (*models.Notebook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if nb, ok := r.notebooks[notebookID]; ok && nb.UserID == userID {
		return nb, nil
	}
	return nil, ErrNotFound
}

func (r *MockNotebookRepository) ListByUser(ctx context.Context, userID string) ([]models.Notebook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []models.Notebook
	for _, nb := range r.notebooks {
		if nb.UserID == userID {
			result = append(result, *nb)
		}
	}
	return result, nil
}

func (r *MockNotebookRepository) Update(ctx context.Context, notebook *models.Notebook) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	notebook.UpdatedAt = time.Now()
	r.notebooks[notebook.ID] = notebook
	return nil
}

func (r *MockNotebookRepository) Delete(ctx context.Context, userID, notebookID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if nb, ok := r.notebooks[notebookID]; !ok || nb.UserID != userID {
		return ErrNotFound
	}
	delete(r.notebooks, notebookID)
	return nil
}

func (r *MockNotebookRepository) AddDocument(ctx context.Context, notebookID, documentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if nb, ok := r.notebooks[notebookID]; ok {
		nb.DocumentCnt++
	}
	return nil
}

func (r *MockNotebookRepository) RemoveDocument(ctx context.Context, notebookID, documentID string) error {
	return nil
}

func (r *MockNotebookRepository) ListDocuments(ctx context.Context, notebookID string) ([]models.Document, error) {
	return nil, nil
}

func (r *MockNotebookRepository) GetDocumentIDs(ctx context.Context, notebookID string) ([]string, error) {
	return []string{}, nil
}

func (r *MockNotebookRepository) UpsertGuide(ctx context.Context, guide *models.DocumentGuide) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.guides[guide.DocumentID] = guide
	return nil
}

func (r *MockNotebookRepository) GetGuide(ctx context.Context, documentID string) (*models.DocumentGuide, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.guides[documentID]; ok {
		return g, nil
	}
	return nil, ErrNotFound
}

func (r *MockNotebookRepository) UpsertArtifact(context.Context, *models.NotebookArtifact) error {
	return nil
}

func (r *MockNotebookRepository) GetArtifact(context.Context, string, string, string) (*models.NotebookArtifact, error) {
	return nil, ErrNotFound
}

func (r *MockNotebookRepository) GetArtifactByID(context.Context, string) (*models.NotebookArtifact, error) {
	return nil, ErrNotFound
}

func (r *MockNotebookRepository) ListArtifacts(context.Context, string, string) ([]models.NotebookArtifact, error) {
	return nil, nil
}

func (r *MockNotebookRepository) DeleteArtifact(context.Context, string, string, string) error {
	return nil
}

// MockNotebookVectorStore implements NotebookVectorStore for testing
type MockNotebookVectorStore struct {
	chunks []struct {
		chunk  repository.NotebookChunk
		vector []float32
	}
}

func NewMockNotebookVectorStore() *MockNotebookVectorStore {
	return &MockNotebookVectorStore{
		chunks: make([]struct {
			chunk  repository.NotebookChunk
			vector []float32
		}, 0),
	}
}

func (m *MockNotebookVectorStore) InsertChunks(ctx context.Context, chunks []repository.NotebookChunk, vectors [][]float32) error {
	for i := range chunks {
		m.chunks = append(m.chunks, struct {
			chunk  repository.NotebookChunk
			vector []float32
		}{chunk: chunks[i], vector: vectors[i]})
	}
	return nil
}

func (m *MockNotebookVectorStore) Search(ctx context.Context, queryVector []float32, topK int, notebookID string, docIDs []string) ([]repository.NotebookChunk, []float32, error) {
	var results []repository.NotebookChunk
	var scores []float32
	for i := 0; i < topK && i < len(m.chunks); i++ {
		results = append(results, m.chunks[i].chunk)
		scores = append(scores, 0.9-float32(i)*0.1)
	}
	return results, scores, nil
}

func (m *MockNotebookVectorStore) DeleteByDocument(ctx context.Context, documentID string) error {
	return nil
}

func (m *MockNotebookVectorStore) DeleteByNotebook(ctx context.Context, notebookID string) error {
	return nil
}

func (m *MockNotebookVectorStore) GetAllChunks(ctx context.Context) ([]repository.NotebookChunk, error) {
	chunks := make([]repository.NotebookChunk, 0, len(m.chunks))
	for _, c := range m.chunks {
		chunks = append(chunks, c.chunk)
	}
	return chunks, nil
}

// MockEmbedder implements embeddings.Embedder for testing
type MockEmbedder struct{}

func (m *MockEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vector := make([]float32, 1536)
	for i := range vector {
		vector[i] = 0.01
	}
	return vector, nil
}

func (m *MockEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = make([]float32, 1536)
		for j := range vectors[i] {
			vectors[i][j] = 0.01
		}
	}
	return vectors, nil
}

// ============ 测试用例 ============

func TestNotebookService_CreateNotebook(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	notebook, err := svc.CreateNotebook(ctx, userID, "测试笔记本", "这是一个测试笔记本")
	if err != nil {
		t.Fatalf("CreateNotebook failed: %v", err)
	}

	if notebook.Title != "测试笔记本" {
		t.Errorf("Expected title '测试笔记本', got '%s'", notebook.Title)
	}

	if notebook.Description != "这是一个测试笔记本" {
		t.Errorf("Expected description '这是一个测试笔记本', got '%s'", notebook.Description)
	}

	if notebook.Status != models.NotebookStatusActive {
		t.Errorf("Expected status '%s', got '%s'", models.NotebookStatusActive, notebook.Status)
	}
}

func TestNotebookService_GetNotebook(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	created, err := svc.CreateNotebook(ctx, userID, "测试笔记本", "")
	if err != nil {
		t.Fatalf("CreateNotebook failed: %v", err)
	}

	notebook, err := svc.GetNotebook(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetNotebook failed: %v", err)
	}

	if notebook.ID != created.ID {
		t.Errorf("Expected ID '%s', got '%s'", created.ID, notebook.ID)
	}
}

func TestNotebookService_GetNotebook_NotFound(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()

	_, err := svc.GetNotebook(ctx, "user", "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent notebook, got nil")
	}
}

func TestNotebookService_ListNotebooks(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	for i := 0; i < 3; i++ {
		_, err := svc.CreateNotebook(ctx, userID, "笔记本", "")
		if err != nil {
			t.Fatalf("CreateNotebook failed: %v", err)
		}
	}

	notebooks, err := svc.ListNotebooks(ctx, userID)
	if err != nil {
		t.Fatalf("ListNotebooks failed: %v", err)
	}

	if len(notebooks) != 3 {
		t.Errorf("Expected 3 notebooks, got %d", len(notebooks))
	}
}

func TestNotebookService_UpdateNotebook(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	notebook, _ := svc.CreateNotebook(ctx, userID, "旧标题", "")

	notebook.Title = "新标题"
	notebook.Description = "新描述"
	err := svc.UpdateNotebook(ctx, notebook)
	if err != nil {
		t.Fatalf("UpdateNotebook failed: %v", err)
	}

	updated, _ := svc.GetNotebook(ctx, userID, notebook.ID)
	if updated.Title != "新标题" {
		t.Errorf("Expected title '新标题', got '%s'", updated.Title)
	}
}

func TestNotebookService_DeleteNotebook(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	notebook, _ := svc.CreateNotebook(ctx, userID, "待删除", "")

	err := svc.DeleteNotebook(ctx, userID, notebook.ID)
	if err != nil {
		t.Fatalf("DeleteNotebook failed: %v", err)
	}

	_, err = svc.GetNotebook(ctx, userID, notebook.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
}

func TestNotebookService_AddDocumentToNotebook(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	notebook, _ := svc.CreateNotebook(ctx, userID, "测试", "")

	err := svc.AddDocumentToNotebook(ctx, notebook.ID, "doc-123")
	if err != nil {
		t.Fatalf("AddDocumentToNotebook failed: %v", err)
	}
}

func TestNotebookService_GetDocumentGuide_NotFound(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()

	_, err := svc.GetDocumentGuide(ctx, "non-existent-doc")
	if err == nil {
		t.Error("Expected error for non-existent guide, got nil")
	}
}

// ============ 向量存储测试 ============

func TestMockNotebookVectorStore_InsertAndSearch(t *testing.T) {
	store := NewMockNotebookVectorStore()
	ctx := context.Background()

	chunks := []repository.NotebookChunk{
		{
			NotebookID: "nb-1",
			DocumentID: "doc-1",
			PageNumber: 1,
			ChunkIndex: 0,
			Content:    "这是第一段内容",
		},
		{
			NotebookID: "nb-1",
			DocumentID: "doc-1",
			PageNumber: 1,
			ChunkIndex: 1,
			Content:    "这是第二段内容",
		},
	}

	vectors := [][]float32{
		make([]float32, 1536),
		make([]float32, 1536),
	}

	err := store.InsertChunks(ctx, chunks, vectors)
	if err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}

	queryVector := make([]float32, 1536)
	results, scores, err := store.Search(ctx, queryVector, 2, "nb-1", nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if len(scores) != 2 {
		t.Errorf("Expected 2 scores, got %d", len(scores))
	}
}

func TestNotebookServiceIndexesParsedChunksWithStructuredVisualMetadata(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	svc := NewNotebookService(repo, store, embedder, nil, &configs.LLMConfig{}, nil)

	err := svc.IndexParsedChunks(context.Background(), "user-1", "notebook-1", "doc-1", []*parser.Chunk{
		{
			ID:         "chunk-1",
			Content:    "[Visual: chart]\nSummary: Revenue grew.\nVisualPath: storage/visual/doc-1/page-1/block-1.png",
			PageNum:    1,
			ChunkIndex: 3,
			ChunkType:  parser.BlockTypeImage,
			BBox:       parser.BoundingBox{X0: 1, Y0: 2, X1: 3, Y1: 4},
			Metadata: map[string]any{
				"chunk_role":  "parent",
				"visual_type": "chart",
			},
		},
	})
	if err != nil {
		t.Fatalf("IndexParsedChunks returned error: %v", err)
	}
	if len(store.chunks) != 1 {
		t.Fatalf("expected one indexed chunk, got %d", len(store.chunks))
	}
	chunk := store.chunks[0].chunk
	if chunk.ChunkType != "image" || chunk.ChunkRole != "parent" {
		t.Fatalf("expected structured chunk metadata, got %#v", chunk)
	}
	if chunk.PageNumber != 0 || chunk.BBox != `[1,2,3,4]` {
		t.Fatalf("expected normalized page/bbox metadata, got page=%d bbox=%q", chunk.PageNumber, chunk.BBox)
	}
}

// ============ 并发测试 ============

func TestNotebookService_ConcurrentAccess(t *testing.T) {
	repo := NewMockNotebookRepository()
	store := NewMockNotebookVectorStore()
	embedder := &MockEmbedder{}
	cfg := &configs.LLMConfig{}

	svc := NewNotebookService(repo, store, embedder, nil, cfg, nil)

	ctx := context.Background()
	userID := "test-user-1"

	var wg sync.WaitGroup
	success := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.CreateNotebook(ctx, userID, "并发笔记本", "")
			if err == nil {
				success <- true
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(success)
	}()

	count := 0
	for range success {
		count++
	}

	if count != 10 {
		t.Errorf("Expected 10 successful creates, got %d", count)
	}
}
