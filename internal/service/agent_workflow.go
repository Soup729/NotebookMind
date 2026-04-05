package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// ============ 智能体工作流 (Map-Reduce) ============

// MapReduceConfig Map-Reduce 配置
type MapReduceConfig struct {
	MaxWorkers     int // 最大并发 worker 数
	ChunkSize      int // 每个 worker 处理的文档数
	SummaryMaxLen  int // 摘要最大长度
	LLMModel       string
}

// MapReduceResult Map-Reduce 结果
type MapReduceResult struct {
	FinalSummary string
	SourceCount  int
	ProcessingMs int64
	Errors       []error
}

// MapReduceService 接口
type MapReduceService interface {
	// SummarizeDocuments 并发总结多个文档
	SummarizeDocuments(ctx context.Context, docs []DocumentInput, prompt string) (*MapReduceResult, error)
}

// DocumentInput 文档输入
type DocumentInput struct {
	ID      string
	Content string
	Meta    map[string]interface{}
}

// MapReduceServiceImpl 实现
type mapReduceServiceImpl struct {
	llm          interface{} // llms.Model
	embedder     interface{} // embeddings.Embedder
	config       *MapReduceConfig
}

// NewMapReduceService 创建 Map-Reduce 服务
func NewMapReduceService(llm, embedder interface{}, config *MapReduceConfig) MapReduceService {
	if config == nil {
		config = &MapReduceConfig{
			MaxWorkers:    4,
			ChunkSize:     5,
			SummaryMaxLen: 500,
		}
	}
	return &mapReduceServiceImpl{
		llm:      llm,
		embedder: embedder,
		config:   config,
	}
}

// SummarizeDocuments 并发总结多文档
func (m *mapReduceServiceImpl) SummarizeDocuments(ctx context.Context, docs []DocumentInput, prompt string) (*MapReduceResult, error) {
	if len(docs) == 0 {
		return &MapReduceResult{}, nil
	}

	startMs := currentTimeMs()
	var wg sync.WaitGroup
	var summaryLock sync.Mutex
	var intermediateResults []string
	var errors []error
	var processedCount int32

	// 控制并发数
	semaphore := make(chan struct{}, m.config.MaxWorkers)

	// 分块处理
	chunkSize := m.config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 5
	}

	// Map 阶段：并发总结每个文档
	mapResults := make(chan string, len(docs))
	mapErrors := make(chan error, len(docs))

	for i, doc := range docs {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, input DocumentInput) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// 单文档总结
			summary, err := m.summarizeSingle(ctx, input, prompt)
			if err != nil {
				mapErrors <- err
				return
			}
			mapResults <- summary
			atomic.AddInt32(&processedCount, 1)
		}(i, doc)
	}

	// 等待所有 Map 任务完成
	go func() {
		wg.Wait()
		close(mapResults)
		close(mapErrors)
	}()

	// 收集 Map 结果
	for {
		select {
		case summary, ok := <-mapResults:
			if !ok {
				mapResults = nil
			} else {
				summaryLock.Lock()
				intermediateResults = append(intermediateResults, summary)
				summaryLock.Unlock()
			}
		case err, ok := <-mapErrors:
			if !ok {
				mapErrors = nil
			} else {
				errors = append(errors, err)
			}
		}

		if mapResults == nil && mapErrors == nil {
			break
		}
	}

	// Reduce 阶段：合并所有总结
	var finalSummary string
	if len(intermediateResults) == 0 {
		finalSummary = "无法生成摘要"
	} else if len(intermediateResults) == 1 {
		finalSummary = intermediateResults[0]
	} else {
		// 合并中间结果
		merged := strings.Join(intermediateResults, "\n\n---\n\n")
		finalSummary, _ = m.generateFinalSummary(ctx, merged, prompt)
	}

	return &MapReduceResult{
		FinalSummary: finalSummary,
		SourceCount:  len(docs),
		ProcessingMs: currentTimeMs() - startMs,
		Errors:       errors,
	}, nil
}

// summarizeSingle 总结单个文档
func (m *mapReduceServiceImpl) summarizeSingle(ctx context.Context, doc DocumentInput, prompt string) (string, error) {
	// 截断过长的文档
	content := doc.Content
	maxLen := m.config.SummaryMaxLen * 4 // 约等于 token 数
	if len(content) > maxLen {
		content = content[:maxLen] + "..."
	}

	// 构建提示词
	summaryPrompt := fmt.Sprintf(`请简要总结以下文档的核心内容：

文档ID: %s
内容:
%s

%s

请用2-3句话总结要点。`, doc.ID, content, prompt)

	// 调用 LLM
	response, err := m.callLLM(ctx, summaryPrompt)
	if err != nil {
		return "", fmt.Errorf("summarize doc %s: %w", doc.ID, err)
	}

	return fmt.Sprintf("[%s] %s", doc.ID, response), nil
}

// generateFinalSummary 生成最终总结
func (m *mapReduceServiceImpl) generateFinalSummary(ctx context.Context, mergedSummary string, originalPrompt string) (string, error) {
	prompt := fmt.Sprintf(`以下是多个文档的摘要，请综合整理成一份完整的总结：

%s

用户原始问题: %s

请生成一份综合摘要，突出多个文档的共同主题和关键信息。`, mergedSummary, originalPrompt)

	return m.callLLM(ctx, prompt)
}

// callLLM 调用 LLM
func (m *mapReduceServiceImpl) callLLM(ctx context.Context, prompt string) (string, error) {
	// 使用 llms.GenerateFromSinglePrompt
	// 这里简化实现
	return "Summary placeholder", nil
}

// currentTimeMs 获取当前时间戳（毫秒）
func currentTimeMs() int64 {
	return int64(0) // 简化
}

// ============ Agent 工作流 ============

// AgentWorkflowConfig Agent 工作流配置
type AgentWorkflowConfig struct {
	MaxSteps        int // 最大思考步数
	TimeoutSeconds  int
	EnableReflection bool // 启用反思机制
}

// AgentStep Agent 思考步骤
type AgentStep struct {
	StepNumber int
	Thought    string
	Action     string
	Observation string
	Result     string
}

// AgentWorkflowResult Agent 工作流结果
type AgentWorkflowResult struct {
	Steps      []AgentStep
	FinalAnswer string
	Success    bool
	Error      error
}

// AgentWorkflow Agent 工作流接口
type AgentWorkflow interface {
	Execute(ctx context.Context, task string, contextDocs []DocumentInput) (*AgentWorkflowResult, error)
}

// agentWorkflow 实现
type agentWorkflow struct {
	llm          interface{}
	hybridSearch HybridSearchService
	mapReduce    MapReduceService
	config       *AgentWorkflowConfig
}

// NewAgentWorkflow 创建 Agent 工作流
func NewAgentWorkflow(
	llm interface{},
	hybridSearch HybridSearchService,
	mapReduce MapReduceService,
	config *AgentWorkflowConfig,
) AgentWorkflow {
	if config == nil {
		config = &AgentWorkflowConfig{
			MaxSteps:        5,
			TimeoutSeconds:  60,
			EnableReflection: true,
		}
	}
	return &agentWorkflow{
		llm:          llm,
		hybridSearch: hybridSearch,
		mapReduce:    mapReduce,
		config:       config,
	}
}

// Execute 执行 Agent 工作流
func (a *agentWorkflow) Execute(ctx context.Context, task string, contextDocs []DocumentInput) (*AgentWorkflowResult, error) {
	result := &AgentWorkflowResult{
		Steps: []AgentStep{},
	}

	zap.L().Info("agent workflow started", zap.String("task", task))

	// 步骤 1: 理解任务
	thought := a.think(task, contextDocs)
	result.Steps = append(result.Steps, AgentStep{
		StepNumber: 1,
		Thought:    thought,
		Action:     "理解任务并制定计划",
	})

	// 步骤 2: 检索相关信息
	var retrievedDocs []DocumentInput
	if a.hybridSearch != nil && len(contextDocs) > 0 {
		searchResults, err := a.hybridSearch.Search(ctx, task, "", nil, 10)
		if err != nil {
			zap.L().Warn("search failed", zap.Error(err))
		} else {
			for _, r := range searchResults {
				retrievedDocs = append(retrievedDocs, DocumentInput{
					ID:      r.DocumentID,
					Content: r.Content,
				})
			}
		}
	}
	result.Steps = append(result.Steps, AgentStep{
		StepNumber: 2,
		Thought:    fmt.Sprintf("检索到 %d 个相关文档", len(retrievedDocs)),
		Action:     "检索相关信息",
		Observation: fmt.Sprintf("找到 %d 个相关块", len(retrievedDocs)),
	})

	// 步骤 3: 整合信息并生成答案
	var answer string
	if len(retrievedDocs) > 0 {
		// 使用 Map-Reduce 处理多文档
		if a.mapReduce != nil {
			mapResult, err := a.mapReduce.SummarizeDocuments(ctx, retrievedDocs, task)
			if err != nil {
				result.Error = err
				result.Success = false
				return result, err
			}
			answer = mapResult.FinalSummary
		}
	} else if len(contextDocs) > 0 {
		// 直接使用提供的文档
		answer = a.generateAnswer(task, contextDocs)
	} else {
		answer = "没有找到相关信息"
	}

	result.Steps = append(result.Steps, AgentStep{
		StepNumber: 3,
		Thought:    "综合信息生成答案",
		Action:     "生成最终答案",
		Result:     answer,
	})

	// 步骤 4: 反思 (可选)
	if a.config.EnableReflection {
		reflection := a.reflect(task, answer)
		result.Steps = append(result.Steps, AgentStep{
			StepNumber: 4,
			Thought:    reflection,
			Action:     "反思答案质量",
		})
	}

	result.FinalAnswer = answer
	result.Success = true

	zap.L().Info("agent workflow completed", zap.String("task", task))
	return result, nil
}

// think 思考
func (a *agentWorkflow) think(task string, docs []DocumentInput) string {
	return fmt.Sprintf("用户问题: %s\n文档数量: %d\n这是一个需要%s的问题。",
		task, len(docs), a.classifyTask(task))
}

// classifyTask 分类任务
func (a *agentWorkflow) classifyTask(task string) string {
	task = strings.ToLower(task)
	if strings.Contains(task, "总结") || strings.Contains(task, "摘要") {
		return "总结"
	}
	if strings.Contains(task, "比较") || strings.Contains(task, "对比") {
		return "比较"
	}
	if strings.Contains(task, "解释") || strings.Contains(task, "定义") {
		return "解释"
	}
	return "回答"
}

// generateAnswer 生成答案
func (a *agentWorkflow) generateAnswer(task string, docs []DocumentInput) string {
	// 简化实现
	var content strings.Builder
	for i, doc := range docs {
		if i > 0 && i%3 == 0 {
			break
		}
		content.WriteString(doc.Content)
		content.WriteString("\n\n")
	}
	return content.String()
}

// reflect 反思
func (a *agentWorkflow) reflect(task, answer string) string {
	// 检查答案质量
	if len(answer) < 20 {
		return "答案太短，可能不完整"
	}
	return "答案看起来完整且相关"
}
