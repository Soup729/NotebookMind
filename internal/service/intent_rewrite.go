package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/repository"

	"github.com/tmc/langchaingo/llms"
	"go.uber.org/zap"
)

var stableSingleAnchorPattern = regexp.MustCompile(`\b(?:[A-Z][A-Za-z]+-[A-Z][A-Za-z]+|[A-Z]{2,}|R&D|AI)\b`)
var stablePercentAnchorPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?%`)

// ============ 查询意图重写服务 (Phase 2 重写版) ============

// QueryIntent 查询意图类型
type QueryIntent string

const (
	IntentFactual    QueryIntent = "factual"    // 事实查询
	IntentSummary    QueryIntent = "summary"    // 总结摘要
	IntentComparison QueryIntent = "comparison" // 比较对比
	IntentAnalysis   QueryIntent = "analysis"   // 分析推理
	IntentDefinition QueryIntent = "definition" // 定义解释
	IntentProcedure  QueryIntent = "procedure"  // 流程步骤
	IntentUnknown    QueryIntent = "unknown"    // 未知
)

// RewriteResult 重写结果
type RewriteResult struct {
	OriginalQuery  string
	RewrittenQuery string
	Intent         QueryIntent
	Keywords       []string
	ContextTerms   []string // 从历史对话中提取的上下文术语
}

// IntentRewriteService 意图重写接口
type IntentRewriteService interface {
	Rewrite(ctx context.Context, userID, sessionID, query string) (*RewriteResult, error)
	IdentifyIntent(query string) QueryIntent
}

// intentRewriteService 实现
type intentRewriteService struct {
	chatRepo  repository.ChatRepository
	llm       llms.Model
	tokenizer *Tokenizer
	config    *configs.IntentRewriteConfig
}

// NewIntentRewriteService 创建意图重写服务
func NewIntentRewriteService(
	chatRepo repository.ChatRepository,
	llm llms.Model,
	tokenizer *Tokenizer,
	config *configs.IntentRewriteConfig,
) IntentRewriteService {
	return &intentRewriteService{
		chatRepo:  chatRepo,
		llm:       llm,
		tokenizer: tokenizer,
		config:    config,
	}
}

// Rewrite 重写查询
// 根据 .env 配置决定行为：
// - Enabled=false → 原样返回
// - Enabled=true, LLMRewriteEnabled=false → 规则意图识别 + 规则重写
// - Enabled=true, LLMRewriteEnabled=true → 规则意图识别 + LLM 重写
func (s *intentRewriteService) Rewrite(ctx context.Context, userID, sessionID, query string) (*RewriteResult, error) {
	contextualQuery, followupTerms := s.rewriteFollowUpWithConversationContext(ctx, userID, sessionID, query)

	// 未启用 → 原样返回
	if !s.config.Enabled {
		return &RewriteResult{
			OriginalQuery:  query,
			RewrittenQuery: contextualQuery,
			Intent:         IntentUnknown,
			ContextTerms:   followupTerms,
		}, nil
	}

	// 1. 规则意图识别（不消耗 token）
	intent := s.IdentifyIntent(contextualQuery)

	// 2. 从历史对话中提取上下文术语（不消耗 token）
	contextTerms := s.extractContextTerms(ctx, userID, sessionID)
	if len(followupTerms) > 0 {
		contextTerms = mergeContextTerms(contextTerms, followupTerms, s.maxContextTerms())
	}

	// 3. 提取关键词（不消耗 token）
	keywords := s.extractKeywords(contextualQuery)

	// 4. 查询重写
	var rewritten string
	var err error

	if s.config.LLMRewriteEnabled && s.llm != nil {
		// LLM 重写（消耗 token）
		rewritten, err = s.rewriteByLLM(ctx, contextualQuery, intent, contextTerms)
		if err != nil {
			zap.L().Warn("LLM rewrite failed, falling back to rule-based rewrite", zap.Error(err))
			rewritten = s.rewriteByRules(contextualQuery, intent, contextTerms)
		}
	} else {
		// 规则重写（不消耗 token）
		rewritten = s.rewriteByRules(contextualQuery, intent, contextTerms)
	}

	return &RewriteResult{
		OriginalQuery:  query,
		RewrittenQuery: rewritten,
		Intent:         intent,
		Keywords:       keywords,
		ContextTerms:   contextTerms,
	}, nil
}

func (s *intentRewriteService) rewriteFollowUpWithConversationContext(ctx context.Context, userID, sessionID, query string) (string, []string) {
	if !isFollowUpQuery(query) || s.chatRepo == nil || sessionID == "" {
		return query, nil
	}

	messages, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil || len(messages) == 0 {
		return query, nil
	}

	var lastUser string
	var lastAssistant string
	for i := len(messages) - 1; i >= 0; i-- {
		content := strings.TrimSpace(messages[i].Content)
		if content == "" {
			continue
		}
		switch messages[i].Role {
		case "assistant":
			if lastAssistant == "" {
				lastAssistant = content
			}
		case "user":
			if lastUser == "" && !strings.EqualFold(content, strings.TrimSpace(query)) {
				lastUser = content
			}
		}
		if lastUser != "" && lastAssistant != "" {
			break
		}
	}

	var contextParts []string
	if lastUser != "" {
		contextParts = append(contextParts, lastUser)
	}
	if lastAssistant != "" {
		contextParts = append(contextParts, lastAssistant)
	}
	if len(contextParts) == 0 {
		return query, nil
	}

	fullContextText := strings.Join(contextParts, " ")
	anchors := stableFactAnchorsFromText(fullContextText, s.maxContextTerms()*2)
	contextText := truncateRunes(fullContextText, 500)
	sections := []string{query}
	if len(anchors) > 0 {
		sections = append(sections, "Previous turn fact anchors: "+strings.Join(anchors, "; "))
	}
	sections = append(sections, "Previous turn context: "+contextText)
	rewritten := strings.TrimSpace(strings.Join(sections, " "))
	return rewritten, mergeContextTerms(extractTermsFromText(s.tokenizer, strings.Join(anchors, " "), s.maxContextTerms()), extractTermsFromText(s.tokenizer, contextText, s.maxContextTerms()), s.maxContextTerms())
}

func isFollowUpQuery(query string) bool {
	normalized := strings.ToLower(query)
	markers := []string{
		"that", "this", "those", "these", "it", "they", "them", "their",
		"among those", "above", "previous", "earlier", "last answer",
		"does that represent", "that regional", "those top",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func (s *intentRewriteService) maxContextTerms() int {
	if s.config != nil && s.config.MaxContextTerms > 0 {
		return s.config.MaxContextTerms
	}
	return 8
}

// IdentifyIntent 识别查询意图（规则匹配，不消耗 token）
func (s *intentRewriteService) IdentifyIntent(query string) QueryIntent {
	normalized := strings.ToLower(query)

	// 使用统一 Tokenizer 分词（而非旧的 tokenize）
	var tokens []string
	if s.tokenizer != nil {
		tokens = s.tokenizer.Tokenize(normalized)
	} else {
		tokens = tokenize(normalized)
	}

	// 同时检查原始 query 中的模式（中文关键词可能被分成单字）
	intentKeywords := map[QueryIntent][]string{
		IntentFactual:    {"what", "who", "when", "where", "which", "多少", "几个", "是谁", "是什么", "是谁", "什么"},
		IntentSummary:    {"总结", "概括", "摘要", "要点", "summarize", "summary", "main points", "概述", "归纳"},
		IntentComparison: {"比较", "对比", "差异", "区别", "compare", "difference", "vs", "versus", "不同", "相同"},
		IntentAnalysis:   {"分析", "原因", "为什么", "如何", "explain", "why", "how", "怎么", "为何"},
		IntentDefinition: {"定义", "什么是", "含义", "definition", "means", "meaning", "意思", "指的"},
		IntentProcedure:  {"步骤", "流程", "如何做", "方法", "steps", "how to", "process", "怎么做", "怎样"},
	}

	// 计算每个意图的匹配度
	scores := make(map[QueryIntent]int)
	for intent, kws := range intentKeywords {
		for _, kw := range kws {
			// 检查原始 query 中是否包含关键词子串
			if strings.Contains(normalized, kw) {
				scores[intent] += 2 // 原始匹配权重更高
			}
			// 检查分词后的 token
			for _, token := range tokens {
				if token == kw || strings.Contains(token, kw) {
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

// extractContextTerms 从历史对话中提取上下文术语（不消耗 token）
func (s *intentRewriteService) extractContextTerms(ctx context.Context, userID, sessionID string) []string {
	if s.chatRepo == nil || sessionID == "" {
		return nil
	}

	messages, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil {
		return nil
	}

	maxTerms := s.config.MaxContextTerms
	if maxTerms <= 0 {
		maxTerms = 3
	}

	var terms []string
	seen := make(map[string]struct{})

	for _, msg := range messages {
		var tokens []string
		if s.tokenizer != nil {
			tokens = s.tokenizer.Tokenize(msg.Content)
		} else {
			tokens = tokenize(msg.Content)
		}
		for _, word := range tokens {
			// 过滤短词（中文保留2字以上，英文保留3字以上）
			runeLen := len([]rune(word))
			if runeLen < 2 {
				continue
			}
			if _, ok := seen[word]; !ok {
				seen[word] = struct{}{}
				terms = append(terms, word)
				if len(terms) >= maxTerms*2 { // 提取多一些候选，后面截断
					break
				}
			}
		}
		if len(terms) >= maxTerms*2 {
			break
		}
	}

	// 截断到配置的最大数量
	if len(terms) > maxTerms {
		terms = terms[:maxTerms]
	}

	return terms
}

func extractTermsFromText(tokenizer *Tokenizer, text string, maxTerms int) []string {
	if maxTerms <= 0 {
		maxTerms = 8
	}
	var tokens []string
	if tokenizer != nil {
		tokens = tokenizer.Tokenize(text)
	} else {
		tokens = tokenize(text)
	}
	terms := make([]string, 0, maxTerms)
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if len([]rune(token)) < 2 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		terms = append(terms, token)
		if len(terms) >= maxTerms {
			break
		}
	}
	return terms
}

func mergeContextTerms(existing, extra []string, maxTerms int) []string {
	if maxTerms <= 0 {
		maxTerms = 8
	}
	merged := make([]string, 0, maxTerms)
	seen := make(map[string]struct{}, maxTerms)
	for _, terms := range [][]string{extra, existing} {
		for _, term := range terms {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = struct{}{}
			merged = append(merged, term)
			if len(merged) >= maxTerms {
				return merged
			}
		}
	}
	return merged
}

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func stableFactAnchorsFromText(text string, maxAnchors int) []string {
	if maxAnchors <= 0 {
		maxAnchors = 12
	}
	seen := make(map[string]struct{}, maxAnchors)
	anchors := make([]string, 0, maxAnchors)
	add := func(anchor string) {
		anchor = strings.Trim(anchor, " \t\r\n.,;:!?()[]{}\"'")
		if anchor == "" {
			return
		}
		key := strings.ToLower(anchor)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		anchors = append(anchors, anchor)
	}

	for _, phrase := range extractEntityPhrases(text) {
		add(phrase)
		if len(anchors) >= maxAnchors {
			return anchors
		}
	}
	for _, phrase := range stableSingleAnchorPattern.FindAllString(text, -1) {
		add(phrase)
		if len(anchors) >= maxAnchors {
			return anchors
		}
	}
	for _, percent := range stablePercentAnchorPattern.FindAllString(text, -1) {
		add(percent)
		if len(anchors) >= maxAnchors {
			return anchors
		}
	}
	for _, number := range extractTrustNumbers(text) {
		add(number)
		if len(anchors) >= maxAnchors {
			return anchors
		}
	}
	lower := strings.ToLower(text)
	for _, phrase := range []string{"product drivers", "regional growth", "revenue growth", "incremental revenue growth"} {
		if strings.Contains(lower, phrase) {
			add(phrase)
			if len(anchors) >= maxAnchors {
				return anchors
			}
		}
	}
	return anchors
}

// rewriteByRules 规则重写（不消耗 token）
func (s *intentRewriteService) rewriteByRules(query string, intent QueryIntent, contextTerms []string) string {
	var rewritten strings.Builder
	rewritten.WriteString(query)

	// 添加上下文术语增强
	if len(contextTerms) > 0 {
		rewritten.WriteString(" ")
		for i := 0; i < len(contextTerms); i++ {
			rewritten.WriteString(contextTerms[i])
			if i < len(contextTerms)-1 {
				rewritten.WriteString(" ")
			}
		}
	}

	// 意图特定优化
	switch intent {
	case IntentSummary:
		rewritten.WriteString(" 请总结核心要点")
	case IntentComparison:
		rewritten.WriteString(" 进行详细对比")
	case IntentAnalysis:
		rewritten.WriteString(" 深入分析原因")
	case IntentDefinition:
		rewritten.WriteString(" 给出精确定义")
	case IntentProcedure:
		rewritten.WriteString(" 列出具体步骤")
	}

	return strings.TrimSpace(rewritten.String())
}

// rewriteByLLM LLM 查询重写（消耗 token）
func (s *intentRewriteService) rewriteByLLM(ctx context.Context, query string, intent QueryIntent, contextTerms []string) (string, error) {
	contextStr := ""
	if len(contextTerms) > 0 {
		contextStr = fmt.Sprintf("\n上下文关键术语: %s", strings.Join(contextTerms, ", "))
	}

	prompt := fmt.Sprintf(`你是一个查询重写助手。请将用户的查询重写为更适合检索的形式。

规则:
1. 保留原始查询的核心意图
2. 扩展同义词和相关术语
3. 补充隐含的上下文信息
4. 使查询更具体、更精确
5. 只输出重写后的查询，不要解释

原始查询: %s
意图类型: %s%s

重写后的查询:`, query, string(intent), contextStr)

	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM rewrite: %w", err)
	}

	rewritten := strings.TrimSpace(response)
	if rewritten == "" {
		return query, nil // 降级为原始查询
	}

	return rewritten, nil
}

// extractKeywords 提取关键词
func (s *intentRewriteService) extractKeywords(query string) []string {
	var tokens []string
	if s.tokenizer != nil {
		tokens = s.tokenizer.Tokenize(query)
	} else {
		tokens = tokenize(strings.ToLower(query))
	}

	// Tokenizer 已经过滤了停用词，直接返回
	var keywords []string
	for _, word := range tokens {
		if len([]rune(word)) >= 2 {
			keywords = append(keywords, word)
		}
	}
	return keywords
}
