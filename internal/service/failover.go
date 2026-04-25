package service

import (
	"context"
	"fmt"

	"NotebookAI/internal/configs"

	"go.uber.org/zap"
)

// ============ Failover 策略 (Phase 2) ============

// FailoverStrategy 检索 failover 策略
type FailoverStrategy struct {
	minConfidenceScore float32
	maxRetries         int
	tokenizer          *Tokenizer
	config             *configs.HybridSearchConfig
}

// FailoverResult Failover 结果
type FailoverResult struct {
	Results     []HybridResult
	Retried     bool
	RetryReason string
}

// NewFailoverStrategy 创建 Failover 策略
func NewFailoverStrategy(config *configs.HybridSearchConfig, tokenizer *Tokenizer) *FailoverStrategy {
	minConf := config.MinConfidence
	if minConf <= 0 {
		minConf = 0.3
	}
	maxRetries := config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	return &FailoverStrategy{
		minConfidenceScore: minConf,
		maxRetries:         maxRetries,
		tokenizer:          tokenizer,
		config:             config,
	}
}

// Check 检查检索结果的置信度，低置信度时触发二次检索策略
// 注意：当前实现只做置信度检查和标记，实际的二次检索由调用者根据返回结果决定
func (f *FailoverStrategy) Check(results []HybridResult) *FailoverResult {
	failoverResult := &FailoverResult{
		Results: results,
		Retried: false,
	}

	if len(results) == 0 {
		failoverResult.RetryReason = "no results returned"
		failoverResult.Retried = true
		return failoverResult
	}

	// 检查 Top-1 分数是否低于阈值
	topScore := results[0].Score
	if topScore < f.minConfidenceScore {
		zap.L().Warn("low confidence retrieval detected",
			zap.Float32("top_score", topScore),
			zap.Float32("threshold", f.minConfidenceScore),
			zap.Int("result_count", len(results)),
		)
		failoverResult.RetryReason = fmt.Sprintf("low confidence score: %.4f < %.4f", topScore, f.minConfidenceScore)
		failoverResult.Retried = true

		// 标记低置信度但仍然返回结果（兜底策略：有结果总比没结果好）
		// 实际的二次检索由 HybridSearchService.Search 方法处理
	}

	return failoverResult
}

// ExpandQuery 扩展查询（用于二次检索）
// 使用 Tokenizer 提取关键 token，构建扩展查询
func (f *FailoverStrategy) ExpandQuery(query string) string {
	if f.tokenizer == nil {
		return query
	}

	tokens := f.tokenizer.Tokenize(query)
	if len(tokens) == 0 {
		return query
	}

	// 返回分词后的 token 拼接（去除停用词后的精华版本）
	expanded := ""
	for _, token := range tokens {
		if expanded != "" {
			expanded += " "
		}
		expanded += token
	}

	if expanded == "" {
		return query
	}
	return expanded
}

// ShouldRetry 判断是否需要二次检索
func (f *FailoverStrategy) ShouldRetry(results []HybridResult, retryCount int) bool {
	if retryCount >= f.maxRetries {
		return false
	}
	if len(results) == 0 {
		return true
	}
	return results[0].Score < f.effectiveMinConfidence(results[0])
}

func (f *FailoverStrategy) effectiveMinConfidence(result HybridResult) float32 {
	if result.Metadata != nil {
		if scoreType, ok := result.Metadata["score_type"].(string); ok && scoreType == "rrf" {
			if rrfHits(result.Metadata) < 2 {
				rrfK := 60
				if f.config != nil && f.config.RRFK > 0 {
					rrfK = f.config.RRFK
				}
				return float32(1.0/float64(rrfK+1)) * 1.25
			}
			rrfK := 60
			if f.config != nil && f.config.RRFK > 0 {
				rrfK = f.config.RRFK
			}
			// RRF scores are rank-fusion scores, not cosine similarities. A single
			// rank-1 hit is around 1/(k+1), so the dense threshold would retry almost
			// every healthy hybrid search.
			return float32(1.0/float64(rrfK+1)) * 0.5
		}
	}
	return f.minConfidenceScore
}

func rrfHits(metadata map[string]interface{}) int {
	if metadata == nil {
		return 0
	}
	switch v := metadata["rrf_hits"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// ExecuteRetry 执行二次检索（使用扩展查询）
func (f *FailoverStrategy) ExecuteRetry(
	ctx context.Context,
	query string,
	searchFn func(ctx context.Context, query string) ([]HybridResult, error),
) ([]HybridResult, error) {
	expandedQuery := f.ExpandQuery(query)
	if expandedQuery == query {
		// 无法扩展查询，直接返回
		return nil, fmt.Errorf("cannot expand query for retry")
	}

	zap.L().Info("executing failover retry with expanded query",
		zap.String("original", query),
		zap.String("expanded", expandedQuery),
	)

	results, err := searchFn(ctx, expandedQuery)
	if err != nil {
		return nil, fmt.Errorf("failover retry failed: %w", err)
	}

	return results, nil
}
