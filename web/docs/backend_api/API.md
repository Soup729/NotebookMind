# Enterprise PDF AI - API 文档

## 概述

Enterprise PDF AI 是一个基于 NotebookLM 理念的企业级 PDF 智能分析平台，支持多文档理解、智能问答、研究笔记管理等功能。

**基础 URL**: `http://localhost:8080/api/v1`

**认证方式**: JWT Bearer Token

---

## 新增功能 API

### 研究笔记 (Notes)

笔记功能允许用户将 AI 回复或原文钉为研究笔记，方便后续查阅和管理。

#### 创建笔记

```
POST /notes
```

**请求体**:
```json
{
  "notebook_id": "可选，关联的笔记本ID",
  "session_id": "可选，关联的聊天会话ID",
  "title": "笔记标题",
  "content": "笔记内容",
  "type": "ai_response | original_text | summary | custom",
  "is_pinned": false,
  "tags": ["标签1", "标签2"],
  "metadata": {
    "source_document_ids": ["doc1", "doc2"]
  }
}
```

**响应**:
```json
{
  "note": {
    "id": "note-uuid",
    "notebook_id": "notebook-uuid",
    "session_id": "session-uuid",
    "title": "笔记标题",
    "content": "笔记内容",
    "type": "ai_response",
    "is_pinned": false,
    "tags": ["标签1", "标签2"],
    "metadata": {},
    "created_at": "2026-04-05T13:00:00Z",
    "updated_at": "2026-04-05T13:00:00Z"
  }
}
```

#### 获取笔记

```
GET /notes/:id
```

#### 更新笔记

```
PUT /notes/:id
```

**请求体**:
```json
{
  "title": "新标题",
  "content": "新内容",
  "is_pinned": true,
  "tags": ["新标签"]
}
```

#### 删除笔记

```
DELETE /notes/:id
```

#### 列出笔记

```
GET /notes
```

**查询参数**:
- `notebook_id`: 按笔记本筛选
- `session_id`: 按会话筛选
- `type`: 按类型筛选
- `tag`: 按标签筛选
- `pinned_only`: 只显示钉住的笔记
- `page`: 页码 (默认 1)
- `page_size`: 每页数量 (默认 20)

#### 钉住/取消钉住

```
POST /notes/:id/pin
```

#### 添加标签

```
POST /notes/:id/tags
```

**请求体**:
```json
{
  "tag": "重要"
}
```

#### 移除标签

```
DELETE /notes/:id/tags
```

**请求体**:
```json
{
  "tag": "重要"
}
```

#### 按标签搜索

```
GET /notes/tags/search?tag=关键词
```

---

## 数据解析层 API

### 文档解析配置

PDF 解析支持 Marker 模型和内置解析器：

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `marker_path` | Marker 可执行文件路径 | 空（使用内置解析）|
| `batch_size` | 批处理大小 | 10 |
| `device_type` | 设备类型 (cuda/cpu/mps) | cpu |
| `max_workers` | 最大并发数 | 4 |

### 表格转换 API

表格支持转换为 Markdown 或 HTML 格式：

- `table_to_markdown(tableHTML)`: HTML → Markdown
- `table_to_html(markdownTable)`: Markdown → HTML

### 坐标系统

文本块包含 bounding box 坐标 `[x0, y0, x1, y1]`，用于前端高亮显示。

### 父子切片策略

- **Child Chunk**: 小粒度块 (默认 512 字符)，用于精准检索
- **Parent Chunk**: 大段落 (5 个 Child)，用于提供完整上下文

---

## 检索增强层 API

### 混合检索

```
POST /notebooks/:id/search
```

**请求体**:
```json
{
  "query": "查询内容",
  "top_k": 5,
  "use_hybrid": true,
  "use_rerank": true
}
```

**响应**:
```json
{
  "query": "查询内容",
  "top_k": 5,
  "items": [
    {
      "chunk_id": "chunk-uuid",
      "document_id": "doc-uuid",
      "content": "检索到的内容",
      "score": 0.95,
      "rank": 1,
      "metadata": {
        "page_number": 1,
        "chunk_index": 0
      }
    }
  ]
}
```

### 检索配置

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `dense_weight` | Dense 向量权重 | 0.7 |
| `sparse_weight` | BM25 权重 | 0.3 |
| `rerank_top_k` | Reranker 返回数量 | 5 |

### 查询意图类型

| 意图 | 说明 | 示例 |
|------|------|------|
| `factual` | 事实查询 | "是什么", "谁", "多少" |
| `summary` | 总结摘要 | "总结要点" |
| `comparison` | 比较对比 | "比较", "差异" |
| `analysis` | 分析推理 | "分析原因" |
| `definition` | 定义解释 | "定义", "什么是" |
| `procedure` | 流程步骤 | "步骤", "如何做" |

---

## 智能体工作流 API

### Map-Reduce 多文档总结

```
POST /notebooks/:id/sessions/:sessionId/map-reduce
```

**请求体**:
```json
{
  "task": "总结所有文档的核心要点",
  "docs": [
    {
      "id": "doc-uuid",
      "content": "文档内容"
    }
  ]
}
```

**响应**:
```json
{
  "final_summary": "综合摘要内容...",
  "source_count": 3,
  "processing_ms": 1234,
  "steps": [
    {
      "step_number": 1,
      "thought": "理解任务",
      "action": "理解任务并制定计划"
    },
    {
      "step_number": 2,
      "thought": "检索到 5 个相关文档",
      "action": "检索相关信息"
    }
  ]
}
```

### Agent 工作流配置

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `max_steps` | 最大思考步数 | 5 |
| `timeout_seconds` | 超时时间 | 60 |
| `enable_reflection` | 启用反思机制 | true |

---

## 企业安全 API

### 多租户隔离

系统支持基于 Milvus Partition Key 的多租户数据隔离。

#### 租户上下文

所有 API 请求自动携带租户隔离信息：
- 用户 ID 作为租户标识
- Milvus 查询自动添加 `user_id` 过滤条件
- 跨租户数据严格不互通

#### 数据访问验证

```
GET /validate-access
```

**响应**:
```json
{
  "valid": true,
  "tenant_id": "tenant-uuid"
}
```

---

## 错误码

| 错误码 | 说明 |
|--------|------|
| 400 | 请求参数错误 |
| 401 | 未认证或 Token 无效 |
| 403 | 无权限访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 示例请求

### cURL 示例

```bash
# 创建笔记
curl -X POST http://localhost:8080/api/v1/notes \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"测试笔记","content":"笔记内容","type":"custom"}'

# 列出笔记
curl -X GET "http://localhost:8080/api/v1/notes?page=1&page_size=10" \
  -H "Authorization: Bearer YOUR_TOKEN"

# 搜索笔记
curl -X GET "http://localhost:8080/api/v1/notes/tags/search?tag=重要" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### JavaScript 示例

```javascript
// 创建笔记
const response = await fetch('/api/v1/notes', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    title: '研究笔记',
    content: 'AI 回复内容...',
    type: 'ai_response',
    tags: ['重要', '待整理']
  })
});
const { note } = await response.json();
```
