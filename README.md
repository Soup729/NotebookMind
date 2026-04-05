# Enterprise PDF AI

企业级多文档智能问答平台，支持 PDF 解析、混合检索、意图识别、SSE 流式响应、视觉问答 (VQA) 和 AI 反思。

## 目录结构

```
enterprise-pdf-ai/
├── cmd/
│   ├── api/              # API 服务入口
│   └── worker/           # 异步 Worker 入口
├── internal/
│   ├── api/
│   │   ├── handlers/     # HTTP 处理器 (Auth, Document, Chat, Search, Note, Dashboard, VQA)
│   │   ├── middleware/   # JWT 认证、日志、限流、CORS
│   │   └── router/       # Gin 路由配置
│   ├── app/              # 依赖注入容器
│   ├── models/           # 数据模型 (User, Document, ChatSession, Note 等)
│   ├── repository/       # 数据访问层 (PostgreSQL, Redis, Milvus)
│   ├── service/          # 业务逻辑层
│   └── worker/           # Asynq 异步任务处理器
├── configs/              # 配置文件
├── api/openapi.yaml      # OpenAPI 规范
├── enterprise-pdf-web/   # Next.js 16 前端
├── scripts/              # 脚本工具 (test_notebook.ps1)
├── docker-compose.yaml   # PostgreSQL + Redis
└── .env.example          # 环境变量模板
```

## 核心能力

### 1. 数据解析层
- **Marker 模型集成**：将 PDF 转换为 Markdown/HTML
- **表格处理**：保留表格结构
- **坐标提取**：边界框 (x0, y0, x1, y1) 存入 Milvus 支持空间检索
- **父子切片策略**：Child 精准检索，Parent 提供完整上下文

### 2. 检索增强层
- **混合检索**：Dense (向量) + Sparse (BM25) 混合，权重可配
- **意图识别**：6 种类型 (factual, summary, comparison, analysis, definition, procedure)
- **查询改写**：结合历史会话提取上下文术语

### 3. 智能体工作流
- **SSE 流式问答**：实时流式输出，支持 8 种事件类型
- **Map-Reduce 并发**：Goroutines 并发总结多文档
- **研究笔记**：创建/更新/删除/钉住/标签管理/按标签搜索
- **AI 反思**：对回答进行准确性、完整性、来源覆盖度分析

### 4. 视觉问答 (VQA)
- **图片上传问答**：上传图片进行问答
- **图片 URL 问答**：通过 URL 获取图片进行问答
- **图文增强问答**：结合文档上下文和图片进行问答

### 5. 企业安全
- **多租户隔离**：Milvus Partition Key 逻辑隔离
- **租户服务**：`TenantIsolation` 接口设计

## 技术栈

| 层级 | 技术 |
|------|------|
| API | Go 1.22 + Gin |
| 数据库 | PostgreSQL + Redis |
| 向量库 | Milvus / Zilliz Cloud |
| 异步任务 | Asynq |
| 前端 | Next.js 16 |
| AI | OpenAI GPT-4o-mini |

## 快速开始

### 1. 启动基础依赖

```powershell
docker compose up -d
```

端口：

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`

### 2. 配置环境变量

```powershell
Copy-Item .env.example .env
```

必须填写：

- `OPENAI_API_KEY`
- `JWT_SECRET`

可选（本地开发可留空）：

- `MILVUS_ADDRESS` / `MILVUS_PASSWORD` - 留空则使用 PostgreSQL 本地向量存储

### 3. 启动后端

```powershell
# 启动 Worker
go run ./cmd/worker

# 启动 API (新窗口)
go run ./cmd/api
```

服务地址：

- API: `http://localhost:8080`
- Base Path: `http://localhost:8080/api/v1`

## API 接口

### 认证
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/auth/register | 注册 |
| POST | /api/v1/auth/login | 登录 |
| GET | /api/v1/me | 当前用户 |

### 文档
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/documents | 文档列表 |
| POST | /api/v1/documents | 上传文档 |
| GET | /api/v1/documents/:id | 文档详情 |
| DELETE | /api/v1/documents/:id | 删除文档 |
| GET | /api/v1/documents/:id/status | 解析状态 |

### 聊天
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/chat/sessions | 会话列表 |
| POST | /api/v1/chat/sessions | 创建会话 |
| GET | /api/v1/chat/sessions/:id/messages | 消息历史 |
| POST | /api/v1/chat/sessions/:id/messages | 发送消息 |
| POST | /api/v1/chat/sessions/:id/stream | SSE 流式问答 |
| POST | /api/v1/chat/sessions/:id/recommendations | 获取推荐问题 |
| POST | /api/v1/chat/sessions/:id/messages/:messageId/reflection | AI 反思 |

### VQA 视觉问答
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/vqa/image | 图片上传问答 |
| POST | /api/v1/vqa/image-url | 图片 URL 问答 |
| POST | /api/v1/vqa/image-context | 图文增强问答 |

### 搜索
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/search?q=... | 混合检索 |

### 笔记
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/notes | 笔记列表 |
| POST | /api/v1/notes | 创建笔记 |
| PUT | /api/v1/notes/:id | 更新笔记 |
| DELETE | /api/v1/notes/:id | 删除笔记 |
| PUT | /api/v1/notes/:id/pin | 钉住/取消钉住 |
| PUT | /api/v1/notes/:id/tags | 更新标签 |
| GET | /api/v1/notes/tags | 所有标签 |
| GET | /api/v1/notes/by-tag/:tag | 按标签搜索 |

### 仪表盘
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/dashboard/overview | 概览统计 |
| GET | /api/v1/usage/summary | 使用量统计 |

## 测试

### 功能测试

```powershell
# 启动 API 服务后，运行测试脚本
.\scripts\test_notebook.ps1 -ApiBase "http://localhost:8080/api/v1"
```

测试覆盖：认证、文档管理、聊天会话、SSE 流式问答、推荐问题、AI 反思、VQA 视觉问答、语义搜索、笔记管理。

### 单元测试

```powershell
go test ./internal/api/handlers
go test ./internal/service
go test ./internal/repository
go test ./internal/worker
```

## 开发说明

### 环境变量

敏感信息通过 `.env` 配置（已加入 `.gitignore`），使用 `.env.example` 作为模板。

### 前端

`enterprise-pdf-web/` 目录为 Next.js 前端，使用 App Router 和 TypeScript。

### 数据库迁移

PostgreSQL 表结构由 GORM AutoMigrate 自动管理，首次启动时会自动创建表。

## Git 工作流

### 合并到 main

```bash
# 1. 确保所有测试通过
.\scripts\test_notebook.ps1 -ApiBase "http://localhost:8080/api/v1"

# 2. 提交当前分支的更改
git add .
git commit -m "feat: 实现 SSE 流式问答、推荐问题、AI 反思、VQA 视觉问答"

# 3. 切换到 main 分支
git checkout main

# 4. 合并功能分支
git merge <分支名>

# 5. 推送 main 分支
git push origin main
```
