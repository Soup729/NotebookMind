package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"enterprise-pdf-ai/internal/models"
	"enterprise-pdf-ai/internal/repository"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"go.uber.org/zap"
)

// NotebookChatSource represents a source chunk for chat response
type NotebookChatSource struct {
	NotebookID   string  `json:"notebook_id"`
	DocumentID   string  `json:"document_id"`
	DocumentName string  `json:"document_name"`
	PageNumber   int64   `json:"page_number"`
	ChunkIndex   int64   `json:"chunk_index"`
	Content      string  `json:"content"`
	Score        float32 `json:"score"`
}

// NotebookChatReply represents a streaming chat response
type NotebookChatReply struct {
	SessionID    string               `json:"session_id"`
	MessageID    string               `json:"message_id"`
	Content      string               `json:"content"`
	Sources      []NotebookChatSource `json:"sources"`
	PromptTokens int                  `json:"prompt_tokens"`
}

// NotebookChatService defines operations for notebook-based chat
type NotebookChatService interface {
	// Session management
	CreateSession(ctx context.Context, userID, notebookID, title string) (*models.ChatSession, error)
	ListSessions(ctx context.Context, userID, notebookID string) ([]models.ChatSession, error)

	// Streaming chat
	StreamChat(ctx context.Context, userID, sessionID, question string, send func(reply *NotebookChatReply) bool) error

	// Search within notebook
	SearchNotebook(ctx context.Context, userID, notebookID, query string, topK int) ([]NotebookChatSource, error)
}

// notebookChatService implements NotebookChatService
type notebookChatService struct {
	notebookRepo  repository.NotebookRepository
	docRepo       repository.DocumentRepository
	vectorStore   repository.NotebookVectorStore
	chatRepo      repository.ChatRepository
	llm           llms.Model
	embedder      embeddings.Embedder
	retrievalTopK int
}

// NewNotebookChatService creates a new NotebookChatService
func NewNotebookChatService(
	notebookRepo repository.NotebookRepository,
	docRepo repository.DocumentRepository,
	vectorStore repository.NotebookVectorStore,
	chatRepo repository.ChatRepository,
	llm llms.Model,
	embedder embeddings.Embedder,
	retrievalTopK int,
) NotebookChatService {
	return &notebookChatService{
		notebookRepo:  notebookRepo,
		docRepo:       docRepo,
		vectorStore:   vectorStore,
		chatRepo:      chatRepo,
		llm:           llm,
		embedder:      embedder,
		retrievalTopK: retrievalTopK,
	}
}

func (s *notebookChatService) CreateSession(ctx context.Context, userID, notebookID, title string) (*models.ChatSession, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New conversation"
	}
	now := time.Now()
	session := &models.ChatSession{
		ID:            uuid.NewString(),
		UserID:        userID,
		NotebookID:   notebookID,
		Title:         title,
		LastMessageAt: now,
	}
	if err := s.chatRepo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func (s *notebookChatService) ListSessions(ctx context.Context, userID, notebookID string) ([]models.ChatSession, error) {
	sessions, err := s.chatRepo.ListSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	// Filter by notebook if needed (implementation depends on how sessions are tagged)
	return sessions, nil
}

// StreamChat performs RAG-based chat with SSE streaming response
func (s *notebookChatService) StreamChat(ctx context.Context, userID, sessionID, question string, send func(reply *NotebookChatReply) bool) error {
	// Step 1: Load session
	session, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Step 2: Get conversation history
	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	// Step 3: Get notebook and document IDs for scope
	docIDs, err := s.notebookRepo.GetDocumentIDs(ctx, session.NotebookID)
	if err != nil {
		return fmt.Errorf("get notebook documents: %w", err)
	}

	// Step 4: Retrieve relevant chunks from Milvus
	// First embed the query
	queryVector, err := s.embedder.EmbedQuery(ctx, question)
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}

	chunks, scores, err := s.vectorStore.Search(ctx, queryVector, s.retrievalTopK, session.NotebookID, docIDs)
	if err != nil {
		return fmt.Errorf("search chunks: %w", err)
	}

	// Step 5: Build sources with document names
	sources := make([]NotebookChatSource, 0, len(chunks))
	for i, chunk := range chunks {
		// Get document name from repository
		doc, _ := s.docRepo.GetByID(ctx, userID, chunk.DocumentID)
		docName := "Unknown Document"
		if doc != nil {
			docName = doc.FileName
		}

		sources = append(sources, NotebookChatSource{
			NotebookID:   chunk.NotebookID,
			DocumentID:   chunk.DocumentID,
			DocumentName: docName,
			PageNumber:   chunk.PageNumber,
			ChunkIndex:   chunk.ChunkIndex,
			Content:      chunk.Content,
			Score:        scores[i],
		})
	}

	// Step 6: Build prompt with strict format
	prompt := s.buildPrompt(history, sources, question)

	// Step 7: Stream LLM response
	messageID := uuid.NewString()

	// Initial response with sources
	initialReply := &NotebookChatReply{
		SessionID:    session.ID,
		MessageID:    messageID,
		Content:      "",
		Sources:      sources,
		PromptTokens: estimateTokens(prompt),
	}
	if !send(initialReply) {
		return fmt.Errorf("client disconnected")
	}

	// Generate response (non-streaming for simplicity, can be extended with streaming)
	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}

	// Send final response
	finalReply := &NotebookChatReply{
		SessionID:    session.ID,
		MessageID:    messageID,
		Content:      strings.TrimSpace(response),
		Sources:      sources,
		PromptTokens: estimateTokens(prompt),
	}
	if !send(finalReply) {
		return fmt.Errorf("client disconnected")
	}

	// Step 8: Save messages to database
	now := time.Now()
	userMessage := &models.ChatMessage{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   question,
		CreatedAt: now,
	}
	if err := s.chatRepo.SaveMessage(ctx, userMessage); err != nil {
		zap.L().Error("save user message failed", zap.Error(err))
	}

	sourcesJSON, _ := json.Marshal(sources)
	assistantMessage := &models.ChatMessage{
		ID:               messageID,
		SessionID:        sessionID,
		UserID:           userID,
		Role:             "assistant",
		Content:          strings.TrimSpace(response),
		SourcesJSON:      string(sourcesJSON),
		PromptTokens:     estimateTokens(prompt),
		CompletionTokens: estimateTokens(response),
		TotalTokens:      estimateTokens(prompt) + estimateTokens(response),
		CreatedAt:        now,
	}
	if err := s.chatRepo.SaveMessage(ctx, assistantMessage); err != nil {
		zap.L().Error("save assistant message failed", zap.Error(err))
	}

	// Update session activity
	title := session.Title
	if title == "" || title == "New conversation" {
		title = buildSessionTitle(question)
	}
	_ = s.chatRepo.UpdateSessionActivity(ctx, userID, sessionID, title, now)

	return nil
}

func (s *notebookChatService) SearchNotebook(ctx context.Context, userID, notebookID, query string, topK int) ([]NotebookChatSource, error) {
	if topK <= 0 {
		topK = s.retrievalTopK
	}

	if s.vectorStore == nil {
		return nil, fmt.Errorf("vector store not available")
	}

	docIDs, err := s.notebookRepo.GetDocumentIDs(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("get notebook documents: %w", err)
	}

	// Embed query
	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	chunks, scores, err := s.vectorStore.Search(ctx, queryVector, topK, notebookID, docIDs)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}

	sources := make([]NotebookChatSource, 0, len(chunks))
	for i, chunk := range chunks {
		doc, _ := s.docRepo.GetByID(ctx, userID, chunk.DocumentID)
		docName := "Unknown Document"
		if doc != nil {
			docName = doc.FileName
		}

		sources = append(sources, NotebookChatSource{
			NotebookID:   chunk.NotebookID,
			DocumentID:   chunk.DocumentID,
			DocumentName: docName,
			PageNumber:   chunk.PageNumber,
			ChunkIndex:   chunk.ChunkIndex,
			Content:      chunk.Content,
			Score:        scores[i],
		})
	}

	return sources, nil
}

// buildPrompt constructs the prompt with strict format for source citations
func (s *notebookChatService) buildPrompt(history []models.ChatMessage, sources []NotebookChatSource, question string) string {
	var builder strings.Builder

	builder.WriteString("You are an enterprise AI assistant similar to Google NotebookLM. ")
	builder.WriteString("Answer questions strictly based on the provided context from documents.\n\n")

	builder.WriteString("## Instructions\n")
	builder.WriteString("1. Answer based ONLY on the provided context\n")
	builder.WriteString("2. When referencing information, cite the source using [Source: DocumentName, Page X]\n")
	builder.WriteString("3. If the context is insufficient, say: 'I cannot find relevant information in the provided documents'\n")
	builder.WriteString("4. Be concise but comprehensive\n\n")

	builder.WriteString("## Conversation History\n")
	if len(history) > 0 {
		for _, msg := range history {
			builder.WriteString(fmt.Sprintf("%s: %s\n", strings.Title(msg.Role), msg.Content))
		}
	} else {
		builder.WriteString("(No previous messages)\n")
	}

	builder.WriteString("\n## Retrieved Context\n")
	for i, src := range sources {
		builder.WriteString(fmt.Sprintf("[%d] Source: %s (Page %d)\nContent: %s\n\n",
			i+1,
			src.DocumentName,
			src.PageNumber+1, // Convert 0-indexed to 1-indexed
			src.Content,
		))
	}

	builder.WriteString("\n## Question\n")
	builder.WriteString(question)
	builder.WriteString("\n\n## Answer\n")

	return builder.String()
}
