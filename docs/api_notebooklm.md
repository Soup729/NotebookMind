# NotebookMind - NotebookLM Core API Documentation

> 基于 Gin + GORM + Milvus SDK + go-openai 构建的类 Google NotebookLM 核心功能 API

## 目录

1. [概述](#概述)
2. [认证](#认证)
3. [笔记本管理 (Notebooks)](#笔记本管理-notebooks)
4. [文档管理 (Documents)](#文档管理-documents)
5. [文档指南 (Document Guide)](#文档指南-document-guide)
6. [会话与流式问答 (Chat)](#会话与流式问答-chat)
7. [错误码](#错误码)

---

## 概述

### Base URL
```
/api/v1
```

### 认证
除 `/auth/*` 端点外，所有接口均需在 Header 中携带 JWT Token：
```
Authorization: Bearer <token>
```

### 通用响应格式
```json
{
  "notebook": { ... },
  "items": [ ... ],
  "error": "错误信息"
}
```

---

## 笔记本管理 (Notebooks)

### 1. 创建笔记本
**POST** `/notebooks`

**请求体：**
```json
{
  "title": "我的笔记本",
  "description": "这是我的第一个笔记本"
}
```

**响应 (201 Created)：**
```json
{
  "notebook": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "title": "我的笔记本",
    "description": "这是我的第一个笔记本",
    "status": "active",
    "document_cnt": 0,
    "created_at": "2026-04-05T12:00:00Z",
    "updated_at": "2026-04-05T12:00:00Z"
  }
}
```

| 字段 | 类型 | 描述 |
|------|------|------|
| title | string | 笔记本标题 (最大255字符) |
| description | string | 笔记本描述 (最大1000字符) |

---

### 2. 获取笔记本
**GET** `/notebooks/:id`

**响应 (200 OK)：**
```json
{
  "notebook": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "title": "我的笔记本",
    "description": "这是我的第一个笔记本",
    "status": "active",
    "document_cnt": 3,
    "created_at": "2026-04-05T12:00:00Z",
    "updated_at": "2026-04-05T12:00:00Z"
  }
}
```

---

### 3. 列出所有笔记本
**GET** `/notebooks`

**响应 (200 OK)：**
```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "我的笔记本",
      "status": "active",
      "document_cnt": 3,
      "created_at": "2026-04-05T12:00:00Z",
      "updated_at": "2026-04-05T12:00:00Z"
    }
  ]
}
```

---

### 4. 更新笔记本
**PUT** `/notebooks/:id`

**请求体：**
```json
{
  "title": "更新后的标题",
  "description": "更新后的描述",
  "status": "archived"
}
```

**响应 (200 OK)：**
```json
{
  "notebook": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "title": "更新后的标题",
    "status": "archived",
    ...
  }
}
```

---

### 5. 删除笔记本
**DELETE** `/notebooks/:id`

**响应：** `204 No Content`

---

## 文档管理 (Documents)

### 6. 添加文档到笔记本
**POST** `/notebooks/:id/documents`

**请求体：**
```json
{
  "document_id": "660e8400-e29b-41d4-a716-446655440001"
}
```

**响应 (200 OK)：**
```json
{
  "message": "document added to notebook"
}
```

---

### 7. 从笔记本移除文档
**DELETE** `/notebooks/:id/documents/:documentId`

**响应：** `204 No Content`

---

### 8. 列出笔记本中的文档
**GET** `/notebooks/:id/documents`

**响应 (200 OK)：**
```json
{
  "items": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "file_name": "年度报告.pdf",
      "status": "completed",
      "file_size": 1024567,
      "chunk_count": 42,
      "created_at": "2026-04-05T12:00:00Z"
    }
  ]
}
```

---

## 文档指南 (Document Guide)

### 9. 获取文档指南
**GET** `/notebooks/:id/documents/:documentId/guide`

> 获取指定文档的自动生成摘要、FAQ 和关键要点

**响应 (200 OK)：**
```json
{
  "guide": {
    "id": "770e8400-e29b-41d4-a716-446655440002",
    "document_id": "660e8400-e29b-41d4-a716-446655440001",
    "summary": "本文档是一份关于2025年公司年度业绩的详细报告...",
    "faq_json": "[{\"question\":\"公司去年的营收增长了多少？\",\"answer\":\"公司去年营收增长了15%...\"}]",
    "key_points": "[\"营收增长15%\", \"海外市场扩展成功\", \"研发投入增加20%\"]",
    "status": "completed",
    "error_msg": null,
    "generated_at": "2026-04-05T12:05:00Z",
    "created_at": "2026-04-05T12:00:00Z"
  }
}
```

**Guide 状态说明：**
| 状态 | 描述 |
|------|------|
| `pending` | 指南生成中 |
| `completed` | 已生成完成 |
| `failed` | 生成失败 |

---

## 会话与流式问答 (Chat)

### 10. 创建聊天会话
**POST** `/notebooks/:id/sessions`

**请求体：**
```json
{
  "title": "关于年度报告的讨论"
}
```

**响应 (201 Created)：**
```json
{
  "session": {
    "id": "880e8400-e29b-41d4-a716-446655440003",
    "user_id": "user123",
    "title": "关于年度报告的讨论",
    "last_message_at": "2026-04-05T12:10:00Z",
    "created_at": "2026-04-05T12:10:00Z"
  }
}
```

---

### 11. 列出笔记本的聊天会话
**GET** `/notebooks/:id/sessions`

**响应 (200 OK)：**
```json
{
  "items": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "title": "关于年度报告的讨论",
      "last_message_at": "2026-04-05T12:15:00Z",
      "created_at": "2026-04-05T12:10:00Z"
    }
  ]
}
```

---

### 11b. 删除聊天会话
**DELETE** `/notebooks/:id/sessions/:sessionId`

> 新增：删除指定笔记本中的某个聊天会话及其所有消息

**响应：** `204 No Content`

| 错误码 | 说明 |
|--------|------|
| 401 | 未认证 |
| 404 | 笔记本或会话不存在 |

---

### 11c. 获取会话消息列表
**GET** `/chat/sessions/:sessionId/messages`

> 获取某个会话的所有历史消息，用于切换会话时加载对话记录

**响应 (200 OK)：**
```json
{
  "items": [
    {
      "id": "990e8400-e29b-41d4-a716-446655440005",
      "session_id": "880e8400-e29b-41d4-a716-446655440003",
      "role": "user",
      "content": "报告中提到的营收增长的主要原因是什么？",
      "sources": [],
      "created_at": "2026-04-05T12:16:00Z"
    },
    {
      "id": "a00e8400-e29b-41d4-a716-446655440007",
      "session_id": "880e8400-e29b-41d4-a716-446655440003",
      "role": "assistant",
      "content": "根据年度报告，营收增长主要得益于...",
      "sources": [
        {
          "document_id": "660e8400-e29b-41d4-a716-446655440001",
          "document_name": "年度报告.pdf",
          "page_number": 3,
          "chunk_index": 12,
          "content": "2024年公司营收达到100亿元，同比增长15%...",
          "score": 0.85
        }
      ],
      "created_at": "2026-04-05T12:17:00Z"
    }
  ]
}
```

**消息字段说明：**
| 字段 | 类型 | 描述 |
|------|------|------|
| id | string | 消息UUID |
| session_id | string | 所属会话ID |
| role | string | 角色: `user` / `assistant` |
| content | string | 消息内容（支持Markdown） |
| sources | array | AI回复的引用来源（用户消息为空数组） |
| created_at | string | 创建时间 (ISO 8601) |

---

### 12. 流式问答 (SSE)
**POST** `/notebooks/:id/sessions/:sessionId/chat`

> 使用 Server-Sent Events (SSE) 进行流式响应，支持引用溯源

**请求体：**
```json
{
  "question": "报告中提到的营收增长的主要原因是什么？"
}
```

**响应类型：** `text/event-stream`

**SSE 事件流格式：**
```
data: {"session_id":"880e8400...","message_id":"990e8400...","content":"","sources":[{"notebook_id":"550e8400...","document_id":"660e8400...","document_name":"年度报告.pdf","page_number":3,"chunk_index":12,"content":"2024年公司营收达到100亿元，同比增长15%...","score":0.85}]}

data: {"session_id":"880e8400...","message_id":"990e8400...","content":"根据","sources":[...]}

data: {"session_id":"880e8400...","message_id":"990e8400...","content":"根据年度报告，","sources":[...]}

data: {"session_id":"880e8400...","message_id":"990e8400...","content":"根据年度报告，营收增长主要得益于...","sources":[...]}

...

data: [DONE]
```

**响应字段说明：**
| 字段 | 类型 | 描述 |
|------|------|------|
| session_id | string | 会话ID |
| message_id | string | 消息ID |
| content | string | 累积的响应内容 |
| sources | array | 引用来源列表 |
| sources[].document_name | string | 来源文档名称 |
| sources[].page_number | int | 来源页码 (1-indexed) |
| sources[].content | string | 来源内容片段 |
| sources[].score | float | 相似度得分 |

---

### 13. 笔记本内搜索
**POST** `/notebooks/:id/search`

> 在笔记本范围内执行向量相似度搜索

**请求体：**
```json
{
  "query": "营收增长",
  "top_k": 5
}
```

**响应 (200 OK)：**
```json
{
  "query": "营收增长",
  "top_k": 5,
  "items": [
    {
      "notebook_id": "550e8400-e29b-41d4-a716-446655440000",
      "document_id": "660e8400-e29b-41d4-a716-446655440001",
      "document_name": "年度报告.pdf",
      "page_number": 3,
      "chunk_index": 12,
      "content": "2024年公司营收达到100亿元，同比增长15%...",
      "score": 0.85
    }
  ]
}
```

---

## 错误码

| HTTP Status | 错误信息 | 描述 |
|--------------|----------|------|
| 400 | Bad Request | 请求参数错误 |
| 401 | Unauthorized | 未认证或Token无效 |
| 403 | Forbidden | 无权限访问该资源 |
| 404 | Not Found | 资源不存在 |
| 500 | Internal Server Error | 服务器内部错误 |

**示例错误响应：**
```json
{
  "error": "notebook not found"
}
```

---

## 数据模型

### Notebook (笔记本)
```go
type Notebook struct {
    ID          string    // UUID
    UserID      string    // 用户ID
    Title       string    // 笔记本标题
    Description string    // 笔记本描述
    Status      string    // active / archived
    DocumentCnt int        // 关联文档数量
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### Document (文档)
```go
type Document struct {
    ID           string    // UUID
    UserID       string    // 用户ID
    NotebookID   string    // 所属笔记本ID
    FileName     string    // 文件名
    StoredPath   string    // 存储路径
    Status       string    // processing / completed / failed
    Summary      string    // 自动生成的摘要
    FaqJSON      string    // 自动生成的FAQ (JSON)
    GuideStatus  string    // pending / completed / failed
    ChunkCount   int       // 分块数量
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### DocumentGuide (文档指南)
```go
type DocumentGuide struct {
    ID          string    // UUID
    DocumentID  string    // 文档ID
    Summary     string    // 文档摘要
    FaqJSON     string    // FAQ (JSON数组)
    KeyPoints   string    // 关键要点 (JSON数组)
    Status      string    // pending / completed / failed
    ErrorMsg    string    // 错误信息
    GeneratedAt time.Time // 生成时间
    CreatedAt   time.Time
}
```

---

## Milvus Schema

**Collection:** `notebook_chunks`

| 字段名 | 类型 | 描述 |
|--------|------|------|
| id | Int64 | 主键 (自增) |
| notebook_id | VarChar(128) | 笔记本ID |
| document_id | VarChar(128) | 文档ID |
| page_number | Int64 | 页码 |
| chunk_index | Int64 | 块索引 |
| content | VarChar(65535) | 文本内容 |
| vector | FloatVector(1536) | 1536维向量 (text-embedding-3-small) |

**索引：** IVF_FLAT, nlist=1024

---

## Prompt 模板

### 流式问答 Prompt
```
You are an enterprise AI assistant similar to Google NotebookLM.
Answer questions strictly based on the provided context from documents.

## Instructions
1. Answer based ONLY on the provided context
2. When referencing information, cite the source using [Source: DocumentName, Page X]
3. If the context is insufficient, say: 'I cannot find relevant information in the provided documents'
4. Be concise but comprehensive

## Conversation History
user: 你好
assistant: 你好，有什么可以帮你的吗？

## Retrieved Context
[1] Source: 年度报告.pdf (Page 3)
Content: 2024年公司营收达到100亿元，同比增长15%...

## Question
报告中提到的营收增长的主要原因是什么？

## Answer
```

### 摘要生成 Prompt
```
You are an AI assistant that generates concise summaries of documents.

Analyze the following document content and provide:
1. A comprehensive summary (2-3 paragraphs)
2. Key topics covered (bullet points)

Document Content:
---
[文档内容]
---

Summary:
```

### FAQ 生成 Prompt
```
You are an AI assistant that generates FAQ sections for documents.

Based on the following document content, generate 5 frequently asked questions and their answers.
Format as JSON array: [{"question": "...", "answer": "..."}]

Document Content:
---
[文档内容]
---

FAQ:
```

---

*文档版本: v1.1.0*
*最后更新: 2026-04-09*
