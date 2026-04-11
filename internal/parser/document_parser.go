package parser

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// DocumentParser 文档解析主入口（编排器）
type DocumentParser struct {
	pdfParser    *PDFParser
	chunkBuilder *ChunkBuilder
	config       *ParserConfig
}

// NewDocumentParser 创建文档解析主入口
func NewDocumentParser(cfg *ParserConfig, ocr OCRProvider, vlm VLMProvider) ParserService {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	pdfParser := NewPDFParser(cfg, ocr, vlm)
	chunkBuilder := NewChunkBuilder(cfg)

	return &DocumentParser{
		pdfParser:     pdfParser,
		chunkBuilder:  chunkBuilder,
		config:        cfg,
	}
}

// ParseDocument 解析文档（实现 ParserService 接口）
func (dp *DocumentParser) ParseDocument(ctx context.Context, filePath string, userID, documentID string) (*ParseResult, error) {
	zap.L().Info("starting document parsing",
		zap.String("document_id", documentID),
		zap.String("file_path", filePath),
		zap.String("user_id", userID),
	)

	result, err := dp.pdfParser.ParseDocument(ctx, filePath, userID, documentID)
	if err != nil {
		return nil, fmt.Errorf("parse document: %w", err)
	}

	return result, nil
}

// BuildChunks 构建父子 chunk（实现 ParserService 接口）
func (dp *DocumentParser) BuildChunks(result *ParseResult, userID, documentID string) ([]*Chunk, []*Chunk) {
	parents, children := dp.chunkBuilder.BuildChunks(result, userID, documentID)

	zap.L().Info("chunk building completed",
		zap.String("document_id", documentID),
		zap.Int("parent_chunks", len(parents)),
		zap.Int("child_chunks", len(children)),
	)

	return parents, children
}

// ParseAndBuild 一步完成解析和分块（便捷方法）
func (dp *DocumentParser) ParseAndBuild(ctx context.Context, filePath, userID, documentID string) (*ParseResult, []*Chunk, []*Chunk, error) {
	result, err := dp.ParseDocument(ctx, filePath, userID, documentID)
	if err != nil {
		return nil, nil, nil, err
	}

	parents, children := dp.BuildChunks(result, userID, documentID)
	return result, parents, children, nil
}
