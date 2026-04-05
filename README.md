# Enterprise PDF AI

企业级多文档智能问答平台，支持 PDF 解析、混合检索、意图识别和 Map-Reduce 并发总结。

## 目录结构

```
enterprise-pdf-ai/
├── cmd/
│   ├── api/              # API 服务入口
│   └── worker/           # 异步 Worker 入口
├── internal/
│   ├── api/
│   │   ├── handlers/     # HTTP 处理器 (Auth, Document, Chat, Search, Note, Dashboard)
│   │   ├── middleware/    # JWT 认证、日志、限流、CORS
│   │   └── router/        # Gin 路由配置
│   ├── app/              # 依赖注入容器
│   ├── models/           # 数据模型 (User, Document, ChatSession, Note 等)
│   ├── repository/       # 数据访问层 (PostgreSQL, Redis, Milvus)
│   ├── service/          # 业务逻辑层
│   └── worker/           # Asynq 异步任务处理器
├── configs/              # 配置文件
├── api/openapi.yaml      # OpenAPI 规范
├── enterprise-pdf-web/   # Next.js 16 前端 (暂未合并)
├── scripts/              # 脚本工具
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
- **Map-Reduce 并发**：Goroutines 并发总结多文档
- **研究笔记**：创建/更新/删除/钉住/标签管理/按标签搜索

### 4. 企业安全
- **多租户隔离**：Milvus Partition Key 逻辑隔离
- **租户服务**：`TenantIsolation` 接口设计

## 技术栈

| 层级 | 技术 |
|------|------|
| API | Go 1.22 + Gin |
| 数据库 | PostgreSQL + Redis |
| 向量库 | Milvus / Zilliz Cloud |
| 异步任务 | Asynq |
| 前端 | Next.js 16 (暂未合并) |
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

`enterprise-pdf-web/` 目录为 Next.js 前端，当前暂未合并到主干。如需开发前端，请单独切换到前端分支。

### 数据库迁移

PostgreSQL 表结构由 GORM AutoMigrate 自动管理，首次启动时会自动创建表。
