package service

import (
	"context"
	"strings"

	"NotebookAI/internal/repository"
)

// ============ 查询意图重写服务 ============

// QueryIntent 查询意图类型
type QueryIntent string

const (
	IntentFactual    QueryIntent = "factual"    // 事实查询
	IntentSummary    QueryIntent = "summary"    // 总结摘要
	IntentComparison QueryIntent = "comparison"  // 比较对比
	IntentAnalysis   QueryIntent = "analysis"   // 分析推理
	IntentDefinition QueryIntent = "definition" // 定义解释
	IntentProcedure  QueryIntent = "procedure"  // 流程步骤
	IntentUnknown    QueryIntent = "unknown"    // 未知
)

// RewriteResult 重写结果
type RewriteResult struct {
	OriginalQuery  string
	RewrittenQuery string
	Intent        QueryIntent
	Keywords      []string
	ContextTerms  []string // 从历史对话中提取的上下文术语
}

// IntentRewriteService 意图重写接口
type IntentRewriteService interface {
	Rewrite(ctx context.Context, userID, sessionID, query string) (*RewriteResult, error)
	IdentifyIntent(query string) QueryIntent
}

// intentRewriteService 实现
type intentRewriteService struct {
	chatRepo repository.ChatRepository
	llm      interface{} // llms.Model
}

// NewIntentRewriteService 创建意图重写服务
func NewIntentRewriteService(chatRepo repository.ChatRepository, llm interface{}) IntentRewriteService {
	return &intentRewriteService{
		chatRepo: chatRepo,
		llm:      llm,
	}
}

// Rewrite 重写查询
func (s *intentRewriteService) Rewrite(ctx context.Context, userID, sessionID, query string) (*RewriteResult, error) {
	// 1. 识别当前查询意图
	intent := s.IdentifyIntent(query)

	// 2. 从历史对话中提取上下文术语
	contextTerms := s.extractContextTerms(ctx, userID, sessionID)

	// 3. 结合意图和上下文重写查询
	rewritten := s.generateRewrite(query, intent, contextTerms)

	// 4. 提取关键词
	keywords := s.extractKeywords(rewritten)

	result := &RewriteResult{
		OriginalQuery:  query,
		RewrittenQuery: rewritten,
		Intent:        intent,
		Keywords:      keywords,
		ContextTerms:  contextTerms,
	}

	return result, nil
}

// IdentifyIntent 识别查询意图
func (s *intentRewriteService) IdentifyIntent(query string) QueryIntent {
	query = strings.ToLower(query)
	words := tokenize(query)

	// 关键词模式匹配
	intentKeywords := map[QueryIntent][]string{
		IntentFactual:    {"what", "who", "when", "where", "which", "多少", "几个", "是谁", "是什么"},
		IntentSummary:    {"总结", "概括", "摘要", "要点", "summarize", "summary", "概括", "main points"},
		IntentComparison: {"比较", "对比", "差异", "区别", "compare", "difference", "vs", "versus"},
		IntentAnalysis:   {"分析", "原因", "为什么", "如何", "分析", "explain", "why", "how"},
		IntentDefinition: {"定义", "什么是", "含义", "definition", "means", "meaning"},
		IntentProcedure:  {"步骤", "流程", "如何做", "方法", "steps", "how to", "process"},
	}

	// 计算每个意图的匹配度
	scores := make(map[QueryIntent]int)
	for intent, kws := range intentKeywords {
		for _, kw := range kws {
			for _, word := range words {
				if strings.Contains(word, kw) {
					scores[intent]++
				}
			}
		}
	}

	// 选择得分最高的意图
	var maxScore int
	var bestIntent QueryIntent = IntentUnknown
	for intent, score := range scores {
		if score > maxScore {
			maxScore = score
			bestIntent = intent
		}
	}

	return bestIntent
}

// extractContextTerms 从历史对话中提取上下文术语
func (s *intentRewriteService) extractContextTerms(ctx context.Context, userID, sessionID string) []string {
	if s.chatRepo == nil {
		return nil
	}

	// 获取最近的消息
	messages, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil {
		return nil
	}

	// 提取名词和实体
	var terms []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		// 简单实现：提取长词作为术语
		words := tokenize(msg.Content)
		for _, word := range words {
			if len(word) > 4 && !seen[word] {
				seen[word] = true
				terms = append(terms, word)
			}
		}
	}

	return terms
}

// generateRewrite 生成重写查询
func (s *intentRewriteService) generateRewrite(query string, intent QueryIntent, contextTerms []string) string {
	// 基于意图重写查询
	var rewritten strings.Builder
	rewritten.WriteString(query)

	// 添加上下文术语增强
	if len(contextTerms) > 0 {
		rewritten.WriteString(" ")
		// 添加最相关的3个上下文术语
		for i := 0; i < 3 && i < len(contextTerms); i++ {
			rewritten.WriteString(contextTerms[i])
			rewritten.WriteString(" ")
		}
	}

	// 意图特定优化
	switch intent {
	case IntentSummary:
		rewritten.WriteString("请总结核心要点")
	case IntentComparison:
		rewritten.WriteString("进行详细对比")
	case IntentAnalysis:
		rewritten.WriteString("深入分析原因")
	case IntentDefinition:
		rewritten.WriteString("给出精确定义")
	case IntentProcedure:
		rewritten.WriteString("列出具体步骤")
	}

	return strings.TrimSpace(rewritten.String())
}

// extractKeywords 提取关键词
func (s *intentRewriteService) extractKeywords(query string) []string {
	words := tokenize(strings.ToLower(query))

	// 停用词
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "can": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "as": true,
		"and": true, "or": true, "but": true, "if": true, "then": true,
		"this": true, "that": true, "these": true, "those": true,
		"的": true, "了": true, "是": true, "在": true, "和": true,
		"有": true, "我": true, "你": true, "他": true, "她": true,
		"它": true, "们": true, "来": true, "去": true, "对": true,
	}

	var keywords []string
	for _, word := range words {
		if len(word) >= 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// QueryExpander 查询扩展器
type QueryExpander struct {
	synonymMap map[string][]string
}

// NewQueryExpander 创建查询扩展器
func NewQueryExpander() *QueryExpander {
	return &QueryExpander{
		synonymMap: map[string][]string{
			"pdf":     {"document", "file", "文档"},
			"ai":      {"artificial intelligence", "人工智能", "machine learning"},
			"search":  {"find", "query", "检索", "查找"},
			"compare": {"contrast", "differentiate", "对比", "区别"},
		},
	}
}

// Expand 扩展查询
func (e *QueryExpander) Expand(query string) string {
	words := tokenize(strings.ToLower(query))
	var expanded []string

	for _, word := range words {
		expanded = append(expanded, word)
		// 添加同义词
		if synonyms, ok := e.synonymMap[word]; ok {
			expanded = append(expanded, synonyms...)
		}
	}

	return strings.Join(expanded, " ")
}
