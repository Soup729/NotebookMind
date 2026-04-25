# NotebookMind API 文档

本文档只描述当前代码中实际注册的 HTTP API，来源为 `internal/api/router/router.go` 与对应 handler。规划接口、内部服务方法和未注册路由不写入本文档。

## 基本信息

- Base URL：`http://localhost:8081/api/v1`
- 认证方式：`Authorization: Bearer <JWT>`
- 公开接口：`GET /ping`、`POST /auth/register`、`POST /auth/login`
- 受保护接口：除以上公开接口外，其余接口都需要 JWT。
- 默认响应格式：成功时返回 JSON 对象；错误时通常返回 `{ "error": "..." }`。

## 端点总览

### 系统与认证

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/ping` | 健康检查 |
| POST | `/auth/register` | 注册 |
| POST | `/auth/login` | 登录 |
| GET | `/me` | 当前用户信息 |

### 仪表盘与用量

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/dashboard/overview` | 仪表盘概览 |
| GET | `/usage/summary` | 用量摘要 |

### 全局文档

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/documents` | 列出当前用户文档 |
| POST | `/documents` | 上传 PDF，可选绑定 notebook |
| GET | `/documents/:id` | 文档详情 |
| DELETE | `/documents/:id` | 删除文档 |

### 通用聊天

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/chat/sessions` | 列出聊天会话 |
| POST | `/chat/sessions` | 创建聊天会话 |
| GET | `/chat/sessions/:id/messages` | 会话消息列表 |
| POST | `/chat/sessions/:id/messages` | 发送普通聊天消息 |
| POST | `/chat/sessions/:id/stream` | SSE 流式聊天 |
| GET | `/chat/models` | 可用模型列表 |
| POST | `/chat/sessions/:id/messages/:messageId/reflection` | 生成消息反思 |

### 全局搜索

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/search?q=...&top_k=5&document_ids=a,b` | 全局文档搜索 |

### Notebook

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/notebooks` | 列出 Notebook |
| POST | `/notebooks` | 创建 Notebook |
| GET | `/notebooks/:id` | Notebook 详情 |
| PUT | `/notebooks/:id` | 更新 Notebook |
| DELETE | `/notebooks/:id` | 删除 Notebook |
| POST | `/notebooks/:id/documents` | 将已有文档加入 Notebook |
| GET | `/notebooks/:id/documents` | 列出 Notebook 文档 |
| DELETE | `/notebooks/:id/documents/:documentId` | 从 Notebook 移除文档 |
| GET | `/notebooks/:id/documents/:documentId/guide` | 获取文档指南 |
| GET | `/notebooks/:id/sessions` | 列出 Notebook 会话 |
| POST | `/notebooks/:id/sessions` | 创建 Notebook 会话 |
| DELETE | `/notebooks/:id/sessions/:sessionId` | 删除 Notebook 会话 |
| GET | `/notebooks/:id/sessions/:sessionId/memory` | 获取会话记忆 |
| POST | `/notebooks/:id/sessions/:sessionId/memory/refresh` | 手动刷新会话记忆 |
| DELETE | `/notebooks/:id/sessions/:sessionId/memory` | 清空会话记忆 |
| POST | `/notebooks/:id/sessions/:sessionId/chat` | Notebook SSE 问答 |
| POST | `/notebooks/:id/search` | Notebook 内检索 |

### Notebook 研究产物与导出

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/notebooks/:id/artifacts` | 列出研究产物 |
| POST | `/notebooks/:id/artifacts/generate` | 生成研究产物 |
| GET | `/notebooks/:id/artifacts/:artifactId` | 获取研究产物 |
| DELETE | `/notebooks/:id/artifacts/:artifactId` | 删除研究产物 |
| POST | `/notebooks/:id/exports/outline` | 生成导出大纲 |
| POST | `/notebooks/:id/exports/:artifactId/confirm` | 确认大纲并提交异步渲染 |
| GET | `/notebooks/:id/exports/:artifactId/download` | 下载导出文件 |

### 研究笔记

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/notes` | 列出笔记 |
| POST | `/notes` | 创建笔记 |
| GET | `/notes/:id` | 笔记详情 |
| PUT | `/notes/:id` | 更新笔记 |
| DELETE | `/notes/:id` | 删除笔记 |
| POST | `/notes/:id/pin` | 切换钉住状态 |
| POST | `/notes/:id/tags` | 添加标签 |
| DELETE | `/notes/:id/tags` | 移除标签 |
| GET | `/notes/tags/search?tag=...` | 按标签搜索 |

### 视觉问答

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/vqa/image` | 上传图片问答 |
| POST | `/vqa/image-url` | 图片 URL 问答 |
| POST | `/vqa/image-context` | 图片 + 文档上下文增强问答 |

## 认证

### POST `/auth/register`

请求：

```json
{
  "name": "Alice",
  "email": "alice@example.com",
  "password": "password123"
}
```

响应：

```json
{
  "token": "jwt",
  "user": {
    "id": "user-id",
    "name": "Alice",
    "email": "alice@example.com"
  }
}
```

### POST `/auth/login`

请求：

```json
{
  "email": "alice@example.com",
  "password": "password123"
}
```

响应同注册。

### GET `/me`

响应：

```json
{
  "user": {
    "id": "user-id",
    "name": "Alice",
    "email": "alice@example.com"
  }
}
```

## 文档

### POST `/documents`

上传 PDF。当前上传接口只接受 `.pdf`，字段名为 `file`。可选 `notebook_id` 会把文档直接关联到指定 Notebook。

请求：`multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| file | File | 是 | PDF 文件 |
| notebook_id | string | 否 | 目标 Notebook ID |

响应状态：`202 Accepted`

```json
{
  "id": "document-id",
  "file_name": "report.pdf",
  "status": "processing",
  "error_message": "",
  "file_size": 102400,
  "chunk_count": 0,
  "task_id": "asynq-task-id",
  "created_at": "2026-04-25T10:00:00Z",
  "updated_at": "2026-04-25T10:00:00Z",
  "processed_at": null
}
```

### GET `/documents`

响应：

```json
{
  "items": [
    {
      "id": "document-id",
      "file_name": "report.pdf",
      "status": "completed",
      "chunk_count": 42
    }
  ]
}
```

### GET `/documents/:id`

返回单个文档元数据。

### DELETE `/documents/:id`

删除文档及其索引数据。成功返回 `204 No Content`。

## Notebook

### POST `/notebooks`

请求：

```json
{
  "title": "年度报告研究",
  "description": "用于分析 2024 年财报"
}
```

响应：

```json
{
  "notebook": {
    "id": "notebook-id",
    "title": "年度报告研究",
    "description": "用于分析 2024 年财报",
    "status": "active",
    "document_cnt": 0,
    "created_at": "2026-04-25T10:00:00Z",
    "updated_at": "2026-04-25T10:00:00Z"
  }
}
```

### GET `/notebooks`

响应：

```json
{
  "items": [
    {
      "id": "notebook-id",
      "title": "年度报告研究",
      "status": "active",
      "document_cnt": 2
    }
  ]
}
```

### GET / PUT / DELETE `/notebooks/:id`

- `GET` 返回 Notebook 详情。
- `PUT` 支持更新 `title`、`description`、`status`，其中 `status` 可为 `active` 或 `archived`。
- `DELETE` 成功返回 `204 No Content`。

### POST `/notebooks/:id/documents`

把已有文档加入 Notebook。上传新文档请使用 `POST /documents`。

请求：

```json
{
  "document_id": "document-id"
}
```

响应：

```json
{
  "message": "document added to notebook"
}
```

### GET `/notebooks/:id/documents`

列出 Notebook 下的文档。

### DELETE `/notebooks/:id/documents/:documentId`

从 Notebook 移除文档。成功返回 `204 No Content`。

### GET `/notebooks/:id/documents/:documentId/guide`

获取文档指南。

```json
{
  "guide": {
    "id": "guide-id",
    "document_id": "document-id",
    "summary": "...",
    "faq_json": "[...]",
    "key_points": "[...]",
    "status": "completed",
    "error_msg": "",
    "generated_at": "2026-04-25T10:00:00Z"
  }
}
```

## Notebook 会话与问答

### POST `/notebooks/:id/sessions`

请求：

```json
{
  "title": "关于风险因素的讨论"
}
```

响应：

```json
{
  "session": {
    "id": "session-id",
    "notebook_id": "notebook-id",
    "title": "关于风险因素的讨论"
  }
}
```

### GET `/notebooks/:id/sessions`

列出 Notebook 会话。

### DELETE `/notebooks/:id/sessions/:sessionId`

删除会话。成功返回 `204 No Content`。

### GET `/notebooks/:id/sessions/:sessionId/memory`

获取当前 Notebook 会话的压缩记忆。该记忆只用于补充多轮上下文，不会覆盖文档证据。

响应：

```json
{
  "memory": {
    "summary": "用户正在比较多份年报中的收入增长解释。",
    "goal": "准备管理层 briefing",
    "decisions": ["优先输出中文结论"],
    "open_questions": ["收入增长是否与风险披露一致"],
    "preferences": ["先给结论，再列证据"],
    "updated_at": "2026-04-25T10:00:00Z"
  }
}
```

### POST `/notebooks/:id/sessions/:sessionId/memory/refresh`

手动刷新会话记忆，适合用户刚完成一段较长讨论后立即进入工作台查看。

响应同获取会话记忆。

### DELETE `/notebooks/:id/sessions/:sessionId/memory`

清空会话记忆。成功返回 `204 No Content`。

### POST `/notebooks/:id/sessions/:sessionId/chat`

Notebook 核心问答接口，返回 `text/event-stream`。

请求：

```json
{
  "question": "这几份文档对收入增长的解释是否一致？",
  "document_ids": ["document-id-1", "document-id-2"]
}
```

SSE 示例：

```text
data: {"session_id":"...","message_id":"...","content":"","sources":[...],"prompt_tokens":123}

data: {"session_id":"...","message_id":"...","content":"根据文档...","sources":[...],"prompt_tokens":123}

data: [DONE]
```

`sources` 字段：

```json
{
  "document_id": "document-id",
  "document_name": "report.pdf",
  "page_number": 0,
  "chunk_index": 7,
  "content": "source text",
  "score": 0.87,
  "chunk_type": "table",
  "section_path": ["Management Discussion"],
  "bounding_box": [10, 20, 200, 240],
  "visual_path": "storage/visual/...",
  "visual_type": "chart"
}
```

说明：

- `page_number` 为后端存储页码，前端展示时通常加 1。
- `bounding_box` 用于 PDF 高亮。
- `visual_path` / `visual_type` 仅在视觉证据存在时返回。

### POST `/notebooks/:id/search`

请求：

```json
{
  "query": "收入趋势图说明了什么？",
  "top_k": 5
}
```

响应：

```json
{
  "query": "收入趋势图说明了什么？",
  "top_k": 5,
  "items": [
    {
      "document_id": "document-id",
      "document_name": "report.pdf",
      "page_number": 0,
      "content": "matched evidence",
      "score": 0.82,
      "chunk_type": "image"
    }
  ],
  "vector": [0.01, 0.02]
}
```

注意：当前实现会返回 query vector，主要用于调试。

## 通用聊天

### POST `/chat/sessions`

创建非 Notebook 聊天会话。

```json
{
  "title": "普通对话"
}
```

### GET `/chat/sessions`

列出通用聊天会话。

### GET `/chat/sessions/:id/messages`

获取消息。

### POST `/chat/sessions/:id/messages`

请求：

```json
{
  "question": "总结这些文档的共同主题",
  "document_ids": ["document-id"]
}
```

响应包含 `session` 和 `message`。

### POST `/chat/sessions/:id/stream`

SSE 流式聊天。事件包括：

- `source`
- `token`
- `done`
- `error`

### GET `/chat/models`

返回当前配置可用模型。

### POST `/chat/sessions/:id/messages/:messageId/reflection`

对指定消息生成反思结果。

## Notebook 研究产物

### POST `/notebooks/:id/artifacts/generate`

生成 Notebook 级结构化研究产物。

请求：

```json
{
  "type": "briefing"
}
```

`type` 可选：

- `briefing`
- `comparison`
- `timeline`
- `topic_clusters`
- `study_pack`

响应：

```json
{
  "artifact": {
    "id": "artifact-id",
    "notebook_id": "notebook-id",
    "type": "briefing",
    "title": "Notebook Briefing",
    "status": "completed",
    "content": {},
    "source_refs": [],
    "version": 1
  }
}
```

### GET `/notebooks/:id/artifacts`

列出研究产物。

### GET `/notebooks/:id/artifacts/:artifactId`

获取研究产物。

### DELETE `/notebooks/:id/artifacts/:artifactId`

删除研究产物。成功返回 `204 No Content`。

## Notebook 导出

导出流程分三步：

1. 创建可编辑大纲。
2. 用户确认或修改大纲。
3. Worker 异步渲染文件，完成后下载。

### POST `/notebooks/:id/exports/outline`

请求：

```json
{
  "format": "pptx",
  "document_ids": ["document-id"],
  "language": "zh-CN",
  "style": "professional",
  "length": "medium",
  "requirements": "面向管理层，突出风险和结论",
  "include_citations": true
}
```

`format` 支持：

- `markdown`
- `mindmap`
- `docx`
- `pptx`
- `pdf`

响应状态：`201 Created`

```json
{
  "artifact": {
    "id": "artifact-id",
    "type": "export_pptx",
    "status": "outline_ready",
    "content": {
      "format": "pptx",
      "outline": [
        {
          "heading": "核心结论",
          "bullets": ["..."]
        }
      ]
    },
    "source_refs": []
  }
}
```

### POST `/notebooks/:id/exports/:artifactId/confirm`

请求：

```json
{
  "outline": [
    {
      "heading": "核心结论",
      "bullets": ["收入增长，利润率承压。"]
    }
  ]
}
```

响应：

```json
{
  "artifact": {
    "id": "artifact-id",
    "status": "generating",
    "task_id": "asynq-task-id"
  }
}
```

### GET `/notebooks/:id/exports/:artifactId/download`

下载已完成文件。若状态不是 `completed`，返回 `409`。

## 研究笔记

### POST `/notes`

请求：

```json
{
  "notebook_id": "notebook-id",
  "session_id": "session-id",
  "title": "关键发现",
  "content": "报告显示收入增长主要来自海外市场。",
  "type": "ai_response",
  "is_pinned": true,
  "tags": ["收入", "海外"],
  "metadata": {
    "source_document_ids": ["document-id"]
  }
}
```

响应：

```json
{
  "note": {
    "id": "note-id",
    "title": "关键发现",
    "content": "报告显示收入增长主要来自海外市场。",
    "tags": ["收入", "海外"]
  }
}
```

### GET `/notes`

查询参数：

| 参数 | 说明 |
| --- | --- |
| notebook_id | 按 Notebook 筛选 |
| session_id | 按会话筛选 |
| type | 按类型筛选 |
| tag | 按标签筛选 |
| pinned_only | 仅钉住 |
| page | 页码，默认 1 |
| page_size | 每页数量，默认 20 |

### GET / PUT / DELETE `/notes/:id`

获取、更新、删除笔记。

### POST `/notes/:id/pin`

切换钉住状态。

### POST `/notes/:id/tags`

请求：

```json
{
  "tag": "重要"
}
```

### DELETE `/notes/:id/tags`

请求同添加标签。

### GET `/notes/tags/search?tag=重要`

按标签搜索笔记。

## VQA 视觉问答

### POST `/vqa/image`

请求：`multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| question | string | 是 | 图片问题 |
| image | File | 是 | JPEG / PNG / GIF / WebP，最大 10MB |

响应：

```json
{
  "answer": "...",
  "prompt_tokens": 100,
  "completion_tokens": 80,
  "total_tokens": 180
}
```

### POST `/vqa/image-url`

请求：

```json
{
  "question": "这张图展示了什么？",
  "image_url": "https://example.com/chart.png"
}
```

### POST `/vqa/image-context`

请求：`multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| question | string | 是 | 图片问题 |
| image | File | 是 | 图片文件 |
| document_ids | JSON string | 否 | 例如 `["doc-1","doc-2"]` |

响应额外包含：

```json
{
  "answer": "...",
  "image_answer": "...",
  "context_enhanced": true
}
```

## Dashboard / Usage / Search

### GET `/dashboard/overview`

返回仪表盘统计。

### GET `/usage/summary`

返回用量摘要。

### GET `/search`

查询参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| q | 是 | 查询文本 |
| top_k | 否 | 1-20，默认 5 |
| document_ids | 否 | 逗号分隔文档 ID |

响应：

```json
{
  "query": "风险因素",
  "items": []
}
```

## 错误码

| 状态码 | 说明 |
| --- | --- |
| 400 | 请求参数错误 |
| 401 | 未认证或 Token 无效 |
| 403 | 无权限 |
| 404 | 资源不存在 |
| 409 | 状态冲突，例如导出未完成 |
| 410 | Notebook 会话已删除或过期 |
| 500 | 服务端错误 |

常见错误响应：

```json
{
  "error": "notebook not found"
}
```

## 未注册为 HTTP API 的能力

以下能力可能存在服务层、计划文档或历史文档中，但当前 router 未注册为 HTTP API：

- `/models` 顶级路由，当前实际为 `GET /chat/models`
- OpenAI 兼容 `/chat/completions`
- `/settings`
- `/knowledge-graph/*`
- `/reflection/think`
- `/reflection/history`
- Notebook MapReduce agent 路由

如需暴露这些能力，应先在 `internal/api/router/router.go` 注册路由并补充 handler。
