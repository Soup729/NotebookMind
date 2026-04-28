# NotebookMind - Testing & Evaluation Guide

## Table of Contents

1. [Directory Structure](#directory-structure)
2. [Offline Evaluation Dataset](#offline-evaluation-dataset)
3. [LLM-as-a-Judge Evaluation Script](#llm-as-a-judge-evaluation-script)
4. [Online Metrics Collection (Structured Logging)](#online-metrics-collection)
5. [Integration Testing](#integration-testing)

---

## Directory Structure

```
tests/
├── data/
│   └── eval_dataset.jsonl    # Offline evaluation dataset (JSONL format, 30 test items)
└── README.md                 # This file
```

> **Note:** Test PDF documents should be placed in `tests/pdf/` or `tmp/uploads/` for evaluation runs.

---

## 1. Offline Evaluation Dataset (`eval_dataset.jsonl`)

### Data Format

Each line is a JSON object with the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique identifier for each test case |
| `category` | string | Yes | Category (see table below) |
| `question` | string | Yes | The user question to send to the API |
| `expected_answer` | string | Yes | Reference answer for Judge evaluation |
| `expected_sources` | string[] | Yes | Expected source documents (used for Recall@K calculation) |
| `difficulty` | string | No | Difficulty level: `easy` / `medium` / `hard` (default: `medium`) |
| `tags` | string[] | No | Descriptive tags for filtering and analysis |
| `turns` | array | No | Multi-turn conversation turns (for `multiturn_followup` category only) |

### Categories and Coverage

The dataset contains **30 test cases** across **12 categories**, designed to comprehensively evaluate every dimension of the RAG system:

| # | Category | Count | Difficulty Distribution | Description |
|---|----------|-------|------------------------|-------------|
| 1 | **factual_qa** | 4 | easy (4) | Direct fact extraction from documents (revenue, employees, leadership) |
| 2 | **table_qa** | 3 | medium (3) | Precise data extraction from structured tables (quarterly revenue, margins, R&D breakdown) |
| 3 | **cross_document_comparison** | 3 | medium (2), hard (1) | Information synthesis across two or more documents |
| 4 | **summary** | 2 | medium (2) | Condensed summarization of document content |
| 5 | **multiturn_followup** | 3 | medium (1), hard (2) | Context retention across conversation turns |
| 6 | **hallucination_check** | 2 | easy (2) | Negative tests -- model must refuse to invent information not in documents |
| 7 | **citation_precision** | 2 | medium (2) | Verification that source citations are accurate and complete |
| 8 | **complex_reasoning** | 2 | hard (2) | Multi-source synthesis requiring analytical reasoning |
| 9 | **procedure** | 2 | medium (2) | Step-by-step process or workflow extraction |
| 10 | **definition** | 2 | medium (2) | Understanding and explaining domain-specific terms/metrics |
| 11 | **analysis** | 2 | medium (2) | Analytical interpretation of financial or operational data |
| 12 | **multimodal_qa** | 3 | hard (3) | Visual understanding -- charts, graphs, diagrams |

### Expected Source Documents

The evaluation dataset references the following mock document names:

| Document Name | Used By Categories | Content Domain |
|---------------|-------------------|----------------|
| `Annual_Report_2024.pdf` | factual_qa, summary, cross_doc, multiturn, hallucination, citation, complex, multimodal | Company overview, financial highlights, strategy, management discussion |
| `Financial_Statements_2024.pdf` | table_qa, cross_doc, definition, analysis, complex, multimodal | Balance sheet, income statement, cash flow, notes |
| `Market_Analysis_2024.pdf` | cross_doc, summary, multiturn, definition, multimodal | Market size, competitive landscape, market share data |
| `HR_Report_2024.pdf` | factual_qa, multiturn | Workforce statistics, headcount, organizational data |
| `Risk_Assessment_2024.pdf` | cross_doc, citation, complex | Risk register, likelihood/impact ratings, mitigation plans |
| `Strategic_Plan_2024.pdf` | citation | Long-term goals, strategic initiatives, roadmap |
| `Process_Guidelines.pdf` | procedure | Internal workflows, approval gates, governance policies |
| `Organization_Chart.pdf` | multimodal | Organizational hierarchy diagram |

### Adding New Test Cases

Append a new line to `tests/data/eval_dataset.jsonl`:

```json
{
  "id": "eval-031",
  "category": "factual_qa",
  "question": "Your question here?",
  "expected_answer": "Expected answer criteria for judge evaluation.",
  "expected_sources": ["SourceDocument.pdf"],
  "difficulty": "medium",
  "tags": ["tag1", "tag2"]
}
```

For multi-turn conversations, include the `turns` field:

```json
{
  "id": "eval-032",
  "category": "multiturn_followup",
  "turns": [
    {"role": "user", "content": "First question?"},
    {"role": "assistant", "content": ""},
    {"role": "user", "content": "Follow-up based on context?"}
  ],
  "question": "Follow-up question here (the last user turn)",
  "expected_answer": "Expected response demonstrating context awareness.",
  "expected_sources": ["Doc1.pdf", "Doc2.pdf"],
  "difficulty": "hard",
  "tags": ["multi-turn", "context"]
}
```

---

## 2. LLM-as-a-Judge Evaluation Script

### Prerequisites

- API service running (`go run ./cmd/api`)
- Worker service running (`go run ./cmd/worker`)
- OpenAI API Key (for the Judge model, can be same as application key)
- Test PDF files placed in `tests/pdf/` or `tmp/uploads/`

### Quick Start

```bash
# Terminal 1 & 2: Start backend services
go run ./cmd/api &
go run ./cmd/worker &

# Terminal 3: Run evaluation (requires OPENAI_API_KEY)
export OPENAI_API_KEY="sk-your-key"
python scripts/notebook_eval.py
```

### Current Evaluation Gate

For Phase 3/4 retrieval and evidence changes, use the local API on port 8081:

```powershell
python scripts/notebook_eval.py --api-base http://localhost:8081/api/v1 -v
```

Keep a change only when the run is at least neutral on overall score and does not regress hallucination rate materially. The latest retained Phase 4 evidence-layer run was:

| Metric | Result |
|--------|--------|
| Overall | 82.6 / 100 |
| Groundedness | 3.93 / 5 |
| Citation Precision | 0.70 / 1 |
| Hallucination Rate | 40.0% |
| Recall@K | 86.9% |
| Table QA Average | 86.0 |

The Phase 3 trust workflow remains disabled by default because the full generation workflow did not pass the same gate when globally enabled.

Phase 5 Notebook research artifacts are currently validated as API smoke and schema tests plus the standard QA regression run. Artifact generation should not change the 30-item QA score unless it shares retrieval or chat code paths.

Phase 6 export MVP is validated separately from chat QA. The required smoke path is: create export outline, confirm outline, wait for the worker to mark the artifact completed, then download the generated file. Current backend coverage includes `markdown`, `mindmap`, `docx`, `pptx`, and `pdf`. Because Phase 6 does not change chat prompts or retrieval, the standard notebook eval should remain neutral.

Citation Guard changes affect the notebook chat prompt and answer post-processing, so run focused service tests before the full evaluator:

```bash
go test ./internal/service -run "TestBuildEvidencePackFromNotebookSources|TestRenderEvidenceCitations|TestValidateCitationBoundAnswer|TestCitationGuard|TestNotebookPromptUsesCanonicalCitationTokensInRetrievedContext" -v
```

Keep Citation Guard changes only when Citation Precision improves materially and hallucination rate does not regress. The current pre-guard checkpoint is Overall 81.1 / 100, Citation Precision 0.58, Hallucination Rate 40.0%, and Recall@K 90.3%.

### Command-Line Arguments

| Argument | Default | Description |
|----------|---------|-------------|
| `--dataset, -d` | `tests/data/eval_dataset.jsonl` | Path to evaluation dataset (JSONL) |
| `--output, -o` | `eval_results_TIMESTAMP.json` | Output report path |
| `--api-base` | `http://localhost:8080/api/v1` | API base URL |
| `--skip-auth` | false | Skip authentication step |
| `--enable-judge` | true | Enable LLM Judge evaluation |
| `--no-judge` | false | Disable Judge (test API availability and latency only) |
| `--judge-model` | `gpt-4o-mini` | Judge model name |
| `--judge-api-key` | `$OPENAI_API_KEY` | Judge API Key |
| `--judge-api-url` | `https://api.openai.com/v1` | Judge API URL |
| `--timeout, -t` | 120 | Request timeout in seconds |
| `--delay` | 0.5 | Delay between requests in seconds |
| `--pdf-dir` | auto-detect | Custom directory for test PDF files |
| `--verbose, -v` | false | Verbose output |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Used as default Judge API Key |
| `EVAL_API_BASE` | Override default API base URL |
| `EVAL_JUDGE_MODEL` | Override default Judge model |
| `EVAL_JUDGE_API_KEY` | Explicit Judge API Key |
| `EVAL_JUDGE_API_URL` | Explicit Judge API URL |
| `NEXT_PUBLIC_API_URL` | Fallback for API URL detection |

### Evaluation Dimensions

The Judge evaluates each AI response on the following dimensions:

| Dimension | Scale | Weight | Description |
|-----------|-------|--------|-------------|
| **Groundedness** | 0-5 | 30% | Is the answer strictly based on provided context without fabrication? |
| **Relevance** | 0-5 | 25% | Does the answer directly address the user's question? |
| **Completeness** | 0-5 | 25% | Does it cover all aspects of the question? |
| **Citation Precision** | 0-1 | 20% | Are source citations accurate and traceable? |
| **Hallucination Detection** | boolean | N/A | Does the answer contain fabricated facts, sources, or logic? |

### Additional Metrics

| Metric | Formula | Range |
|--------|---------|-------|
| **Recall@K** | Matched expected sources / Total expected sources | [0-1] |
| **Overall Score** | G*0.30 + R*0.25 + C*0.25 + CP*20 | [0-100] |
| **Hallucination Rate** | Items with hallucination / Total judged items | [0-100%] |
| **P95 Latency** | 95th percentile of response times | milliseconds |

### Output Report Example

```json
{
  "run_id": "abc12345",
  "timestamp": "2026-04-17T15:00:00",
  "total_items": 30,
  "success_count": 29,
  "error_count": 1,
  "avg_latency_ms": 2345.6,
  "p95_latency_ms": 5234.0,
  "avg_groundedness": 4.2,
  "avg_relevance": 4.5,
  "avg_completeness": 3.8,
  "avg_citation_precision": 0.85,
  "hallucination_rate": 0.07,
  "avg_recall_at_k": 0.73,
  "avg_overall_score": 78.3,
  "category_stats": {
    "factual_qa": { "count": 4, "avg_score": 85.2 },
    "table_qa": { "count": 3, "avg_score": 76.8 }
  },
  "results": [
    {
      "item_id": "eval-001",
      "category": "factual_qa",
      "question": "...",
      "actual_answer": "...",
      "groundedness_score": 5.0,
      "relevance_score": 5.0,
      "completeness_score": 4.0,
      "citation_precision": 0.9,
      "has_hallucination": false,
      "overall_score": 87.5,
      "recall_at_k": 1.0,
      "latency_ms": 1856
    }
  ]
}
```

---

## 3. Online Metrics Collection (Structured Logging)

### Instrumentation Points

| Service File | Event Types | Collected Metrics |
|--------------|------------|-------------------|
| `chat_service.go` | `chat_request`, `retrieval`, `llm_call` | Total latency, retrieval time, LLM time, token usage, source count |
| `notebook_chat_service.go` | `chat_request`, `retrieval`, `llm_call` | Same as above + notebook-specific fields |
| `notebook_service.go` | `document_processing`, `llm_call` | Document processing stages, guide generation time |
| `worker/processor.go` | `document_processing` | PDF extraction, chunking, embedding, indexing stage times |

### Log Entry Example

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
  "@timestamp": "2026-04-17T15:12:34.567Z"
}
```

### Weekly Report Template

Aggregate logs using ELK Stack / Loki / Datadog to generate weekly metrics:

| Metric | This Week | Last Week | Change |
|--------|-----------|-----------|--------|
| Recall@5 | 73% | 68% | +5% |
| Groundedness | 4.2 / 5 | 4.0 / 5 | +0.2 |
| Citation Precision | 85% | 82% | +3% |
| Hallucination Rate | 7% | 12% | -5% |
| P95 Latency | 5.2s | 4.8s | +0.4s |
| Token Cost / Query | 1800 | 1950 | -150 |

---

## 4. Integration Testing

### PowerShell (Windows)

```powershell
.\scripts\test_notebook.ps1 -ApiBase "http://localhost:8080/api/v1"
```

### Bash (Linux/macOS)

```bash
./scripts/test_notebook.sh
```

### Coverage Areas

The integration test script covers:
- Authentication (register, login, JWT token validation)
- Notebook CRUD operations
- Document upload, listing, removal, and status polling
- Chat session creation, message history, and SSE streaming
- Recommended questions endpoint
- AI reflection trigger
- VQA (visual question answering) endpoints
- Semantic search within notebooks
- Note creation, update, delete, pin, tag management

---

## 5. Evaluator Unit Tests

The evaluator has a small Python unit test suite for behavior that should not depend on a running backend:

```powershell
python -m unittest tests.test_notebook_eval -v
```

Covered behaviors:

- JSONL compatibility for both `expected_answer` and the legacy typo `expectedAnswer`.
- Multi-turn items replay every `user` turn in order instead of evaluating only the final question.

When adding new dataset fields, update both `scripts/notebook_eval.py` and these tests so evaluator regressions are caught before long-running LLM judge evaluations.
