package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"NotebookAI/internal/models"
	"NotebookAI/internal/parser"
	"NotebookAI/internal/repository"
	"github.com/tmc/langchaingo/embeddings"
	"go.uber.org/zap"
)

type KnowledgeGraphService interface {
	UpdateDocumentGraph(ctx context.Context, userID, notebookID, documentID string, chunks []*parser.Chunk) error
	DeleteDocumentGraph(ctx context.Context, documentID string) error
	DeleteNotebookGraph(ctx context.Context, notebookID string) error
	GetNotebookGraph(ctx context.Context, userID, notebookID string) (*KnowledgeGraphResponse, error)
	ReindexNotebookGraph(ctx context.Context, userID, notebookID string) error
}

type KnowledgeGraphResponse struct {
	Status              string               `json:"status"`
	SemanticIndexStatus string               `json:"semantic_index_status"`
	Version             int                  `json:"version"`
	Nodes               []KnowledgeGraphNode `json:"nodes"`
	Edges               []KnowledgeGraphEdge `json:"edges"`
	Stats               KnowledgeGraphStats  `json:"stats"`
	Error               string               `json:"error,omitempty"`
	SemanticIndexError  string               `json:"semantic_index_error,omitempty"`
}

type KnowledgeGraphNode struct {
	ID          string                   `json:"id"`
	Label       string                   `json:"label"`
	Type        string                   `json:"type"`
	Size        int                      `json:"size"`
	Confidence  float64                  `json:"confidence"`
	Description string                   `json:"description,omitempty"`
	Documents   []KnowledgeGraphDocument `json:"documents"`
	Evidence    []KnowledgeGraphEvidence `json:"evidence"`
}

type KnowledgeGraphEdge struct {
	ID         string                   `json:"id"`
	Source     string                   `json:"source"`
	Target     string                   `json:"target"`
	Label      string                   `json:"label"`
	Weight     int                      `json:"weight"`
	Confidence float64                  `json:"confidence"`
	Documents  []KnowledgeGraphDocument `json:"documents"`
	Evidence   []KnowledgeGraphEvidence `json:"evidence"`
}

type KnowledgeGraphDocument struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type KnowledgeGraphEvidence struct {
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name,omitempty"`
	Page         int64  `json:"page,omitempty"`
	Text         string `json:"text"`
	ChunkID      string `json:"chunk_id,omitempty"`
	ChunkType    string `json:"chunk_type,omitempty"`
	BBox         string `json:"bbox,omitempty"`
}

type KnowledgeGraphStats struct {
	Entities  int `json:"entities"`
	Relations int `json:"relations"`
	Documents int `json:"documents"`
}

type knowledgeGraphService struct {
	graphRepo     repository.NotebookGraphRepository
	notebookRepo  repository.NotebookRepository
	embedder      embeddings.Embedder
	semanticIndex KnowledgeGraphSemanticIndex
}

func NewKnowledgeGraphService(graphRepo repository.NotebookGraphRepository, notebookRepo repository.NotebookRepository, embedder embeddings.Embedder, semanticIndex KnowledgeGraphSemanticIndex) KnowledgeGraphService {
	if semanticIndex == nil {
		semanticIndex = NewNoopKnowledgeGraphSemanticIndex()
	}
	return &knowledgeGraphService{
		graphRepo:     graphRepo,
		notebookRepo:  notebookRepo,
		embedder:      embedder,
		semanticIndex: semanticIndex,
	}
}

func (s *knowledgeGraphService) UpdateDocumentGraph(ctx context.Context, userID, notebookID, documentID string, chunks []*parser.Chunk) error {
	if s == nil || s.graphRepo == nil {
		return nil
	}
	currentVersion := 0
	if state, err := s.graphRepo.GetState(ctx, notebookID); err == nil {
		currentVersion = state.Version
	}
	_ = s.graphRepo.UpsertState(ctx, &models.NotebookGraphState{
		NotebookID:          notebookID,
		Status:              models.GraphStatusBuilding,
		SemanticIndexStatus: s.semanticIndexStatus(),
		Version:             currentVersion,
	})

	items := extractGraphItems(notebookID, documentID, chunks)
	if err := s.embedItems(ctx, items); err != nil {
		zap.L().Warn("knowledge graph embeddings skipped", zap.String("document_id", documentID), zap.Error(err))
	}

	if err := s.graphRepo.ReplaceDocumentItems(ctx, notebookID, documentID, items); err != nil {
		_ = s.graphRepo.UpsertState(ctx, &models.NotebookGraphState{
			NotebookID:          notebookID,
			Status:              models.GraphStatusFailed,
			SemanticIndexStatus: s.semanticIndexStatus(),
			LastError:           err.Error(),
		})
		return err
	}

	semanticStatus, semanticErr := s.syncSemanticIndex(ctx, items)
	response, err := s.GetNotebookGraph(ctx, userID, notebookID)
	if err != nil {
		return err
	}
	state := &models.NotebookGraphState{
		NotebookID:          notebookID,
		Status:              models.GraphStatusReady,
		SemanticIndexStatus: semanticStatus,
		Version:             currentVersion + 1,
		EntityCount:         response.Stats.Entities,
		RelationCount:       response.Stats.Relations,
		LastError:           "",
		SemanticIndexError:  "",
		UpdatedAt:           time.Now(),
	}
	if semanticErr != nil {
		state.SemanticIndexError = semanticErr.Error()
	}
	if len(items) == 0 && response.Stats.Entities == 0 {
		state.Status = models.GraphStatusEmpty
	}
	return s.graphRepo.UpsertState(ctx, state)
}

func (s *knowledgeGraphService) DeleteDocumentGraph(ctx context.Context, documentID string) error {
	if s == nil || s.graphRepo == nil || strings.TrimSpace(documentID) == "" {
		return nil
	}
	if s.semanticIndex != nil {
		if err := s.semanticIndex.DeleteByDocument(ctx, documentID); err != nil {
			zap.L().Warn("delete graph semantic index by document failed", zap.String("document_id", documentID), zap.Error(err))
		}
	}
	return s.graphRepo.DeleteDocumentItems(ctx, documentID)
}

func (s *knowledgeGraphService) DeleteNotebookGraph(ctx context.Context, notebookID string) error {
	if s == nil || s.graphRepo == nil || strings.TrimSpace(notebookID) == "" {
		return nil
	}
	if s.semanticIndex != nil {
		if err := s.semanticIndex.DeleteByNotebook(ctx, notebookID); err != nil {
			zap.L().Warn("delete graph semantic index by notebook failed", zap.String("notebook_id", notebookID), zap.Error(err))
		}
	}
	return s.graphRepo.DeleteNotebookItems(ctx, notebookID)
}

func (s *knowledgeGraphService) ReindexNotebookGraph(ctx context.Context, userID, notebookID string) error {
	if _, err := s.notebookRepo.GetByID(ctx, userID, notebookID); err != nil {
		return err
	}
	items, err := s.graphRepo.ListNotebookItems(ctx, notebookID)
	if err != nil {
		return err
	}
	status, syncErr := s.syncSemanticIndex(ctx, items)
	state, stateErr := s.graphRepo.GetState(ctx, notebookID)
	if stateErr != nil {
		state = &models.NotebookGraphState{NotebookID: notebookID, Status: models.GraphStatusReady}
	}
	state.SemanticIndexStatus = status
	state.SemanticIndexError = ""
	if syncErr != nil {
		state.SemanticIndexError = syncErr.Error()
	}
	if err := s.graphRepo.UpsertState(ctx, state); err != nil {
		return err
	}
	return syncErr
}

func (s *knowledgeGraphService) GetNotebookGraph(ctx context.Context, userID, notebookID string) (*KnowledgeGraphResponse, error) {
	if _, err := s.notebookRepo.GetByID(ctx, userID, notebookID); err != nil {
		return nil, err
	}

	items, err := s.graphRepo.ListNotebookItems(ctx, notebookID)
	if err != nil {
		return nil, err
	}

	state, stateErr := s.graphRepo.GetState(ctx, notebookID)
	status := models.GraphStatusReady
	version := 0
	lastErr := ""
	semanticStatus := s.semanticIndexStatus()
	semanticErr := ""
	if stateErr == nil {
		status = state.Status
		version = state.Version
		lastErr = state.LastError
		if strings.TrimSpace(state.SemanticIndexStatus) != "" {
			semanticStatus = state.SemanticIndexStatus
		}
		semanticErr = state.SemanticIndexError
	} else if len(items) == 0 {
		status = models.GraphStatusEmpty
	}

	docNames := map[string]string{}
	docs, _ := s.notebookRepo.ListDocuments(ctx, notebookID)
	for _, doc := range docs {
		docNames[doc.ID] = doc.FileName
	}

	nodes, edges, stats := aggregateGraphItems(items, docNames)
	if len(nodes) == 0 && status != models.GraphStatusBuilding && status != models.GraphStatusFailed {
		status = models.GraphStatusEmpty
	}

	return &KnowledgeGraphResponse{
		Status:              status,
		SemanticIndexStatus: semanticStatus,
		Version:             version,
		Nodes:               nodes,
		Edges:               edges,
		Stats:               stats,
		Error:               lastErr,
		SemanticIndexError:  semanticErr,
	}, nil
}

func (s *knowledgeGraphService) semanticIndexStatus() string {
	if s == nil || s.semanticIndex == nil {
		return models.GraphSemanticIndexDisabled
	}
	return s.semanticIndex.Status()
}

func (s *knowledgeGraphService) syncSemanticIndex(ctx context.Context, items []models.NotebookGraphItem) (string, error) {
	if s == nil || s.semanticIndex == nil {
		return models.GraphSemanticIndexDisabled, nil
	}
	status := s.semanticIndex.Status()
	if status == models.GraphSemanticIndexDisabled {
		return status, nil
	}
	if err := s.semanticIndex.UpsertItems(ctx, items); err != nil {
		return models.GraphSemanticIndexFailed, err
	}
	return models.GraphSemanticIndexReady, nil
}

func (s *knowledgeGraphService) embedItems(ctx context.Context, items []models.NotebookGraphItem) error {
	if s.embedder == nil || len(items) == 0 {
		return nil
	}
	texts := make([]string, 0, len(items))
	indexes := make([]int, 0, len(items))
	for i := range items {
		text := strings.TrimSpace(items[i].VectorText)
		if text == "" {
			continue
		}
		texts = append(texts, text)
		indexes = append(indexes, i)
	}
	if len(texts) == 0 {
		return nil
	}
	vectors, err := s.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return err
	}
	for i, vector := range vectors {
		if i >= len(indexes) {
			break
		}
		encoded, _ := json.Marshal(vector)
		items[indexes[i]].EmbeddingJSON = string(encoded)
	}
	return nil
}

type graphCandidate struct {
	Name        string
	Type        string
	Description string
	Confidence  float64
}

func extractGraphItems(notebookID, documentID string, chunks []*parser.Chunk) []models.NotebookGraphItem {
	selected := selectGraphChunks(chunks, 14)
	entityByID := map[string]models.NotebookGraphItem{}
	relations := map[string]models.NotebookGraphItem{}

	for _, chunk := range selected {
		candidates := extractChunkCandidates(chunk)
		if len(candidates) == 0 {
			continue
		}
		if len(candidates) > 5 {
			candidates = candidates[:5]
		}
		for _, candidate := range candidates {
			canonicalID := canonicalEntityID(candidate.Type, candidate.Name)
			if canonicalID == "" {
				continue
			}
			item := models.NotebookGraphItem{
				ID:           deterministicID(documentID, chunk.ID, canonicalID),
				NotebookID:   notebookID,
				DocumentID:   documentID,
				ItemType:     models.GraphItemTypeEntity,
				CanonicalID:  canonicalID,
				EntityName:   candidate.Name,
				EntityType:   candidate.Type,
				DisplayLabel: candidate.Name,
				Description:  candidate.Description,
				EvidenceText: trimEvidence(chunk.Content, 320),
				PageNumber:   int64(chunk.PageNum),
				ChunkID:      chunk.ID,
				ChunkType:    string(chunk.ChunkType),
				BBox:         graphBBoxJSON(chunk.BBox),
				Confidence:   candidate.Confidence,
				Weight:       1,
				VectorText:   fmt.Sprintf("%s %s %s %s", candidate.Name, candidate.Type, candidate.Description, trimEvidence(chunk.Content, 180)),
				MetadataJSON: metadataJSON(chunk),
			}
			if existing, ok := entityByID[canonicalID]; ok {
				existing.Weight++
				if len(existing.EvidenceText) < len(item.EvidenceText) {
					existing.EvidenceText = item.EvidenceText
					existing.PageNumber = item.PageNumber
					existing.ChunkID = item.ChunkID
					existing.ChunkType = item.ChunkType
					existing.BBox = item.BBox
				}
				if item.Confidence > existing.Confidence {
					existing.Confidence = item.Confidence
				}
				entityByID[canonicalID] = existing
			} else {
				entityByID[canonicalID] = item
			}
		}

		source := canonicalEntityID(candidates[0].Type, candidates[0].Name)
		for i := 1; i < len(candidates) && i <= 3; i++ {
			target := canonicalEntityID(candidates[i].Type, candidates[i].Name)
			if source == "" || target == "" || source == target {
				continue
			}
			relID := canonicalRelationID(source, "mentions", target)
			rel := models.NotebookGraphItem{
				ID:             deterministicID(documentID, chunk.ID, relID),
				NotebookID:     notebookID,
				DocumentID:     documentID,
				ItemType:       models.GraphItemTypeRelation,
				CanonicalID:    relID,
				RelationType:   "mentions",
				SourceEntityID: source,
				TargetEntityID: target,
				DisplayLabel:   "mentions",
				EvidenceText:   trimEvidence(chunk.Content, 320),
				PageNumber:     int64(chunk.PageNum),
				ChunkID:        chunk.ID,
				ChunkType:      string(chunk.ChunkType),
				BBox:           graphBBoxJSON(chunk.BBox),
				Confidence:     0.7,
				Weight:         1,
				VectorText:     fmt.Sprintf("%s mentions %s. Evidence: %s", candidates[0].Name, candidates[i].Name, trimEvidence(chunk.Content, 180)),
				MetadataJSON:   metadataJSON(chunk),
			}
			if existing, ok := relations[relID]; ok {
				existing.Weight++
				if rel.Confidence > existing.Confidence {
					existing.Confidence = rel.Confidence
				}
				relations[relID] = existing
			} else {
				relations[relID] = rel
			}
		}
	}

	items := make([]models.NotebookGraphItem, 0, len(entityByID)+len(relations))
	for _, item := range entityByID {
		items = append(items, item)
	}
	for _, item := range relations {
		if _, ok := entityByID[item.SourceEntityID]; ok {
			if _, ok := entityByID[item.TargetEntityID]; ok {
				items = append(items, item)
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ItemType == items[j].ItemType {
			return items[i].CanonicalID < items[j].CanonicalID
		}
		return items[i].ItemType < items[j].ItemType
	})
	return items
}

func selectGraphChunks(chunks []*parser.Chunk, limit int) []*parser.Chunk {
	selected := make([]*parser.Chunk, 0, limit)
	for _, chunk := range chunks {
		if chunk == nil || strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		role, _ := chunk.Metadata["chunk_role"].(string)
		if chunk.ParentID != "" || role == "child" {
			continue
		}
		selected = append(selected, chunk)
		if len(selected) >= limit {
			return selected
		}
	}
	for _, chunk := range chunks {
		if len(selected) >= limit {
			break
		}
		if chunk == nil || chunk.ParentID == "" || strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		selected = append(selected, chunk)
	}
	return selected
}

var (
	acronymPattern       = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,12}\b`)
	titlePhrasePattern   = regexp.MustCompile(`\b[A-Z][A-Za-z0-9]+(?:[-\s][A-Z][A-Za-z0-9]+){0,4}\b`)
	chinesePhrasePattern = regexp.MustCompile(`[\p{Han}]{2,12}`)
)

func extractChunkCandidates(chunk *parser.Chunk) []graphCandidate {
	seen := map[string]bool{}
	candidates := make([]graphCandidate, 0, 8)
	add := func(name, typ, desc string, confidence float64) {
		name = cleanEntityName(name)
		if !validEntityName(name) {
			return
		}
		key := normalizeGraphName(name) + ":" + typ
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, graphCandidate{Name: name, Type: typ, Description: desc, Confidence: confidence})
	}

	for _, section := range chunk.SectionPath {
		add(section, "concept", "section heading", 0.86)
	}
	if chunk.ChunkType == parser.BlockTypeHeading {
		add(chunk.Content, "concept", "document heading", 0.9)
	}
	if chunk.ChunkType == parser.BlockTypeTable {
		add(firstNonEmptyString(graphMetadataString(chunk.Metadata, "table_caption"), sectionTail(chunk.SectionPath), "Table on page "+strconv.Itoa(chunk.PageNum)), "metric", "table or structured data", 0.82)
	}
	if chunk.ChunkType == parser.BlockTypeImage {
		visualType := firstNonEmptyString(graphMetadataString(chunk.Metadata, "visual_type"), "image")
		add(firstNonEmptyString(graphMetadataString(chunk.Metadata, "caption"), sectionTail(chunk.SectionPath), visualType+" on page "+strconv.Itoa(chunk.PageNum)), "concept", "visual evidence", 0.78)
	}

	text := trimEvidence(chunk.Content, 1600)
	for _, match := range acronymPattern.FindAllString(text, 10) {
		add(match, inferEntityType(match, chunk), "acronym or named term", 0.76)
	}
	for _, match := range titlePhrasePattern.FindAllString(text, 14) {
		add(match, inferEntityType(match, chunk), "named term", 0.72)
	}
	for _, match := range chinesePhrasePattern.FindAllString(text, 18) {
		if isMostlyStopPhrase(match) {
			continue
		}
		add(match, inferEntityType(match, chunk), "中文关键概念", 0.7)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Confidence == candidates[j].Confidence {
			return len(candidates[i].Name) > len(candidates[j].Name)
		}
		return candidates[i].Confidence > candidates[j].Confidence
	})
	return candidates
}

func aggregateGraphItems(items []models.NotebookGraphItem, docNames map[string]string) ([]KnowledgeGraphNode, []KnowledgeGraphEdge, KnowledgeGraphStats) {
	type nodeAgg struct {
		node       KnowledgeGraphNode
		docSet     map[string]KnowledgeGraphDocument
		confidence float64
		confCount  int
	}
	nodes := map[string]*nodeAgg{}
	for _, item := range items {
		if item.ItemType != models.GraphItemTypeEntity || item.CanonicalID == "" {
			continue
		}
		agg, ok := nodes[item.CanonicalID]
		if !ok {
			agg = &nodeAgg{
				node: KnowledgeGraphNode{
					ID:          item.CanonicalID,
					Label:       firstNonEmptyString(item.DisplayLabel, item.EntityName, item.CanonicalID),
					Type:        firstNonEmptyString(item.EntityType, "concept"),
					Description: item.Description,
				},
				docSet: map[string]KnowledgeGraphDocument{},
			}
			nodes[item.CanonicalID] = agg
		}
		agg.node.Size += maxInt(1, item.Weight)
		agg.confidence += item.Confidence
		agg.confCount++
		agg.docSet[item.DocumentID] = KnowledgeGraphDocument{ID: item.DocumentID, Name: docNames[item.DocumentID]}
		if len(agg.node.Evidence) < 3 && strings.TrimSpace(item.EvidenceText) != "" {
			agg.node.Evidence = append(agg.node.Evidence, evidenceFromItem(item, docNames))
		}
	}

	type edgeAgg struct {
		edge       KnowledgeGraphEdge
		docSet     map[string]KnowledgeGraphDocument
		confidence float64
		confCount  int
	}
	edges := map[string]*edgeAgg{}
	for _, item := range items {
		if item.ItemType != models.GraphItemTypeRelation || item.SourceEntityID == "" || item.TargetEntityID == "" {
			continue
		}
		if nodes[item.SourceEntityID] == nil || nodes[item.TargetEntityID] == nil {
			continue
		}
		agg, ok := edges[item.CanonicalID]
		if !ok {
			agg = &edgeAgg{
				edge: KnowledgeGraphEdge{
					ID:     item.CanonicalID,
					Source: item.SourceEntityID,
					Target: item.TargetEntityID,
					Label:  firstNonEmptyString(item.DisplayLabel, item.RelationType, "mentions"),
				},
				docSet: map[string]KnowledgeGraphDocument{},
			}
			edges[item.CanonicalID] = agg
		}
		agg.edge.Weight += maxInt(1, item.Weight)
		agg.confidence += item.Confidence
		agg.confCount++
		agg.docSet[item.DocumentID] = KnowledgeGraphDocument{ID: item.DocumentID, Name: docNames[item.DocumentID]}
		if len(agg.edge.Evidence) < 2 && strings.TrimSpace(item.EvidenceText) != "" {
			agg.edge.Evidence = append(agg.edge.Evidence, evidenceFromItem(item, docNames))
		}
	}

	nodeList := make([]KnowledgeGraphNode, 0, len(nodes))
	for _, agg := range nodes {
		if agg.confCount > 0 {
			agg.node.Confidence = round2(agg.confidence / float64(agg.confCount))
		}
		agg.node.Documents = sortedDocs(agg.docSet)
		nodeList = append(nodeList, agg.node)
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].Size == nodeList[j].Size {
			return nodeList[i].Label < nodeList[j].Label
		}
		return nodeList[i].Size > nodeList[j].Size
	})
	if len(nodeList) > 80 {
		nodeList = nodeList[:80]
	}

	allowedNodes := map[string]bool{}
	for _, node := range nodeList {
		allowedNodes[node.ID] = true
	}
	edgeList := make([]KnowledgeGraphEdge, 0, len(edges))
	for _, agg := range edges {
		if !allowedNodes[agg.edge.Source] || !allowedNodes[agg.edge.Target] {
			continue
		}
		if agg.confCount > 0 {
			agg.edge.Confidence = round2(agg.confidence / float64(agg.confCount))
		}
		agg.edge.Documents = sortedDocs(agg.docSet)
		edgeList = append(edgeList, agg.edge)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		if edgeList[i].Weight == edgeList[j].Weight {
			return edgeList[i].ID < edgeList[j].ID
		}
		return edgeList[i].Weight > edgeList[j].Weight
	})
	if len(edgeList) > 120 {
		edgeList = edgeList[:120]
	}

	docSet := map[string]bool{}
	for _, item := range items {
		if item.DocumentID != "" {
			docSet[item.DocumentID] = true
		}
	}
	return nodeList, edgeList, KnowledgeGraphStats{
		Entities:  len(nodeList),
		Relations: len(edgeList),
		Documents: len(docSet),
	}
}

func evidenceFromItem(item models.NotebookGraphItem, docNames map[string]string) KnowledgeGraphEvidence {
	return KnowledgeGraphEvidence{
		DocumentID:   item.DocumentID,
		DocumentName: docNames[item.DocumentID],
		Page:         item.PageNumber,
		Text:         item.EvidenceText,
		ChunkID:      item.ChunkID,
		ChunkType:    item.ChunkType,
		BBox:         item.BBox,
	}
}

func sortedDocs(docSet map[string]KnowledgeGraphDocument) []KnowledgeGraphDocument {
	docs := make([]KnowledgeGraphDocument, 0, len(docSet))
	for _, doc := range docSet {
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool {
		return firstNonEmptyString(docs[i].Name, docs[i].ID) < firstNonEmptyString(docs[j].Name, docs[j].ID)
	})
	return docs
}

func canonicalEntityID(entityType, name string) string {
	normalized := normalizeGraphName(name)
	if normalized == "" {
		return ""
	}
	return "entity:" + firstNonEmptyString(entityType, "concept") + ":" + normalized
}

func canonicalRelationID(source, relationType, target string) string {
	if source == "" || target == "" {
		return ""
	}
	return "rel:" + source + ":" + firstNonEmptyString(relationType, "mentions") + ":" + target
}

func deterministicID(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeGraphName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func cleanEntityName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " \t\r\n:：,，.。;；()（）[]【】{}<>《》\"'")
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 48 {
		runes := []rune(value)
		value = string(runes[:48])
	}
	return value
}

func validEntityName(value string) bool {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) < 2 || len(runes) > 48 {
		return false
	}
	letters := 0
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.Is(unicode.Han, r) {
			letters++
		}
	}
	return letters >= 2 && !isMostlyStopPhrase(value)
}

func isMostlyStopPhrase(value string) bool {
	normalized := normalizeGraphName(value)
	if normalized == "" {
		return true
	}
	stop := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "this": true, "that": true,
		"figure": true, "table": true, "page": true, "section": true, "chapter": true,
		"可以": true, "以及": true, "因此": true, "其中": true, "由于": true, "通过": true,
		"这个": true, "这些": true, "进行": true, "实现": true, "包括": true, "数据": true,
	}
	return stop[normalized]
}

func inferEntityType(name string, chunk *parser.Chunk) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "model") || strings.Contains(lower, "algorithm") || strings.Contains(lower, "method") || strings.Contains(lower, "transformer") {
		return "method"
	}
	if strings.Contains(lower, "dataset") || strings.Contains(lower, "benchmark") {
		return "dataset"
	}
	if strings.Contains(lower, "accuracy") || strings.Contains(lower, "latency") || strings.Contains(lower, "recall") || strings.Contains(lower, "precision") || chunk.ChunkType == parser.BlockTypeTable {
		return "metric"
	}
	if strings.Contains(lower, "inc") || strings.Contains(lower, "corp") || strings.Contains(lower, "university") || strings.Contains(lower, "company") {
		return "org"
	}
	return "concept"
}

func graphMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key]; ok {
		return fmt.Sprint(value)
	}
	return ""
}

func metadataJSON(chunk *parser.Chunk) string {
	if chunk == nil || len(chunk.Metadata) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(chunk.Metadata)
	return string(encoded)
}

func graphBBoxJSON(bbox parser.BoundingBox) string {
	if bbox == (parser.BoundingBox{}) {
		return ""
	}
	encoded, _ := json.Marshal([]float32{bbox.X0, bbox.Y0, bbox.X1, bbox.Y1})
	return string(encoded)
}

func sectionTail(sections []string) string {
	if len(sections) == 0 {
		return ""
	}
	return sections[len(sections)-1]
}

func trimEvidence(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func round2(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}
