# NotebookMind - 测试与评估

## 目录结构

```
tests/
└── data/
    └── eval_dataset.jsonl    # 离线评测集 (JSONL 格式)
```

---

## 1. 离线评测数据集 (`eval_dataset.jsonl`)

### 数据格式

每行一个 JSON 对象，包含以下字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 测试项唯一标识 |
| `category` | string | 分类：factual_qa / cross_document_comparison / table_qa / multimodal_qa / summary / multiturn_followup / hallucination_check / citation_precision / complex_reasoning / procedure |
| `question` | string | 用户问题 |
| `expected_answer` | string | 参考答案（用于 Judge 评估） |
| `expected_sources` | string[] | 期望的来源文档列表（计算 Recall@K） |
| `difficulty` | string | 难度：easy / medium / hard |
| `tags` | string[] | 标签 |
| `turns` | array | 多轮对话内容（可选） |

### 覆盖场景

当前评测集覆盖 **15 条**测试用例，涵盖：

| 场景类别 | 数量 | 示例 |
|---------|------|------|
| **事实问答** (factual_qa) | 2 | "公司2024年营收是多少？" |
| **跨文档对比** (cross_document_comparison) | 2 | "对比两年的研发投入变化" |
| **表格问答** (table_qa) | 2 | "各产品线的毛利率分别是多少？" |
| **图文问答** (multimodal_qa) | 2 | "组织架构图显示的结构？" |
| **摘要生成** (summary) | 1 | "总结年度报告核心要点" |
| **多轮对话** (multiturn_followup) | 2 | 基于上下文的追问 |
| **幻觉检测** (hallucination_check) | 1 | 检测是否编造不存在的信息 |
| **引用精确度** (citation_precision) | 1 | 验证引用准确性 |
| **复杂推理** (complex_reasoning) | 1 | 综合多维度信息分析 |
| **流程提取** (procedure) | 1 | 步骤/流程类问题 |

### 添加新测试用例

在 `tests/data/eval_dataset.jsonl` 中追加一行 JSON：

```json
{
  "id": "eval-016",
  "category": "factual_qa",
  "question": "你的问题",
  "expected_answer": "期望的答案要点",
  "expected_sources": ["相关文档.pdf"],
  "difficulty": "medium",
  "tags": ["标签1", "标签2"]
}
```

---

## 2. LLM-as-a-Judge 评估脚本

### 快速开始

```bash
# 确保服务已运行
go run ./cmd/api & go run ./cmd/worker &

# 运行评估（需要 OPENAI_API_KEY）
export OPENAI_API_KEY="sk-your-key"
python scripts/notebook_eval.py
```

### 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--dataset, -d` | `tests/data/eval_dataset.jsonl` | 评测集路径 |
| `--output, -o` | `eval_results_TIMESTAMP.json` | 输出报告路径 |
| `--api-base` | `http://localhost:8080/api/v1` | API 基础 URL |
| `--enable-judge` | True | 启用 LLM Judge 评估 |
| `--no-judge` | False | 仅测 API 可用性 |
| `--judge-model` | `gpt-4o-mini` | Judge 模型 |
| `--judge-api-key` | `$OPENAI_API_KEY` | Judge API Key |
| `--timeout, -t` | 120s | 单请求超时 |

### 评估输出示例

```json
{
  "run_id": "abc12345",
  "timestamp": "2026-04-11T15:00:00",
  "total_items": 15,
  "success_count": 14,
  "avg_latency_ms": 2345.6,
  "p95_latency_ms": 5234.0,
  "avg_groundedness": 4.2,
  "avg_relevance": 4.5,
  "avg_completeness": 3.8,
  "avg_citation_precision": 0.85,
  "hallucination_rate": 0.07,
  "avg_recall_at_k": 0.73,
  "avg_overall_score": 78.3,
  "category_stats": { ... },
  "results": [ ... ]
}
```

---

## 3. 在线指标采集

### 结构化日志埋点位置

| 服务文件 | 埋点事件 | 采集指标 |
|---------|---------|---------|
| `chat_service.go` | `chat_request`, `retrieval`, `llm_call` | 总延迟、检索耗时、LLM 耗时、Token 用量、Source 数量 |
| `notebook_chat_service.go` | `chat_request`, `retrieval`, `llm_call` | 同上 + Notebook 特有字段 |
| `notebook_service.go` | `document_processing`, `llm_call` | 文档各阶段处理耗时、Guide 生成耗时 |
| `worker/processor.go` | `document_processing` | PDF 提取、分块、Embed、索引各阶段耗时 |

### 日志输出示例

```json
{
  "level": "info",
  "msg": "chat request completed",
  "event": "chat_request",
  "request_id": "uuid-xxx",
  "session_id": "uuid-yyy",
  "service_type": "notebook_chat",
  "total_latency_ms": 2345,
  "retrieval_latency_ms": 156,
  "llm_latency_ms": 2189,
  "retrieved_chunks": 5,
  "top_k": 5,
  "prompt_tokens": 1234,
  "completion_tokens": 567,
  "total_tokens": 1801,
  "source_count": 3,
  "source_document_ids": ["doc-001", "doc-002"],
  "@timestamp": "2026-04-11T15:12:34.567Z"
}
```

### 日志分析建议

使用 ELK Stack / Loki / Datadog 等工具聚合日志后，可以生成：

**周报模板：**

| 指标 | 本周 | 上周 | 变化 |
|------|------|------|------|
| Recall@5 | 73% | 68% | +5% |
| Groundedness | 4.2/5 | 4.0/5 | +0.2 |
| Citation Precision | 85% | 82% | +3% |
| Hallucination Rate | 7% | 12% | -5% |
| P95 Latency | 5.2s | 4.8s | +0.4s |
| Token 成本/Query | 1800 | 1950 | -150 |

---

## 4. 功能集成测试

```powershell
# PowerShell
.\scripts\test_notebook.ps1 -ApiBase "http://localhost:8080/api/v1"

# Bash
./scripts/test_notebook.sh
```
