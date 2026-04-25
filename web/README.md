# NotebookMind - 前端 (Web)

基于 **Next.js 16** 的企业级多文档智能问答平台前端，采用 App Router 架构，支持 SSE 流式对话、PDF 预览、来源定位等核心功能。

---

## 技术栈

| 类别 | 技术 | 版本 |
|------|------|------|
| 框架 | Next.js (App Router) | ^16.2.2 |
| 语言 | TypeScript | ^5.5.4 |
| 样式 | Tailwind CSS | ^3.4.7 |
| 状态管理 | Zustand | ^4.5.4 |
| 数据请求 | SWR | ^2.2.5 |
| UI 组件 | Radix UI + shadcn/ui 风格 | - |
| 图标 | Lucide React | ^0.424.0 |
| Markdown 渲染 | react-markdown + remark-gfm | ^9.0.1 |
| PDF 渲染 | react-pdf + pdfjs-dist | ^9.1.0 / ^4.4.168 |
| 通知 | Sonner | ^1.5.0 |

---

## 目录结构

```
web/
├── src/
│   ├── app/                      # Next.js App Router 页面
│   │   ├── (auth)/               # 认证路由组
│   │   │   ├── login/page.tsx    # 登录页
│   │   │   ├── register/page.tsx # 注册页
│   │   │   └── layout.tsx        # 认证布局
│   │   ├── notebooks/
│   │   │   ├── page.tsx          # 笔记本列表（首页）
│   │   │   └── [id]/             # 笔记本详情（动态路由）
│   │   ├── layout.tsx            # 根布局
│   │   ├── page.tsx              # 首页重定向
│   │   └── globals.css           # 全局样式
│   ├── components/
│   │   ├── chat/                 # 聊天相关组件
│   │   │   ├── ChatPanel.tsx     # 聊天面板（主容器）
│   │   │   ├── ChatMessage.tsx   # 单条消息渲染
│   │   │   └── SourceBadge.tsx   # 来源引用标签
│   │   ├── export/               # Notebook 导出组件（弹窗、大纲编辑、任务托盘）
│   │   ├── guide/                # 文档指南组件
│   │   ├── layout/               # 布局组件（侧边栏、顶栏）
│   │   ├── pdf/                  # PDF 预览与高亮组件
│   │   └── ui/                   # 基础 UI 组件库
│   │       ├── button.tsx        # 按钮
│   │       ├── card.tsx          # 卡片
│   │       ├── input.tsx         # 输入框
│   │       ├── textarea.tsx      # 多行输入框
│   │       ├── avatar.tsx        # 头像
│   │       ├── badge.tsx         # 徽章
│   │       ├── checkbox.tsx      # 复选框
│   │       ├── dropdown-menu.tsx # 下拉菜单
│   │       ├── scroll-area.tsx   # 滚动区域
│   │       ├── skeleton.tsx      # 骨架屏
│   │       └── tooltip.tsx       # 提示框
│   ├── hooks/                    # 自定义 Hooks
│   │   ├── useChat.ts            # 聊天 SSE 流式通信 Hook
│   │   ├── useNotebook.ts        # 笔记本 CRUD 操作 Hook
│   │   └── useNotes.ts           # 研究笔记管理 Hook
│   ├── store/                    # 状态管理
│   │   └── useNotebookStore.ts   # 全局笔记本状态 (Zustand)
│   ├── lib/
│   │   └── utils.ts              # 工具函数（API 请求、SSE 解析等）
│   └── types/
│       ├── api.d.ts              # API 相关类型定义
│       └── pdf.d.ts              # PDF 相关类型定义
├── .env.example                  # 环境变量模板
├── .env.local                    # 本地环境变量（不提交）
├── next.config.mjs               # Next.js 配置
├── tailwind.config.ts            # Tailwind CSS 配置
├── tsconfig.json                 # TypeScript 配置
├── postcss.config.mjs             # PostCSS 配置
└── package.json                  # 项目依赖
```

---

## 架构设计

### 路由架构 (App Router)

```
/                          → 重定向到 /notebooks
├── /login                 → 登录页
├── /register              → 注册页
└── /notebooks
    ├── /notebooks         → 笔记本列表（首页）
    └── /notebooks/[id]    → 笔记本工作台（聊天 + PDF + 指南）
```

### 状态管理 (Zustand)

采用 **Zustand** 进行全局状态管理，核心 Store 为 `useNotebookStore`：

```typescript
// 核心状态
interface NotebookState {
  currentNotebookId: string | null;  // 当前笔记本 ID
  activeSessionId: string | null;    // 当前会话 ID
  selectedDocumentIds: string[];     // 选中的文档 ID 列表
  mainView: 'guide' | 'pdf';        // 主视图模式
  activePdfId: string | null;        // 当前预览的 PDF ID
  highlightTarget: HighlightTarget | null;  // 高亮目标（来源定位）
  selectedModel: string | null;       // LLM 模型选择 (null = 使用默认模型)
}
```

- 使用 `persist` 中间件持久化 `currentNotebookId` 和 `activeSessionId` 到 localStorage
- 使用 `devtools` 中间件支持 Redux DevTools 调试
- 导出 Selector Hooks 和 Action Hooks，避免不必要的重渲染

### 数据请求层

- **SWR**：用于常规 CRUD 操作（笔记本、笔记等），自带缓存和自动重新验证
- **SSE (Server-Sent Events)**：用于聊天流式响应，通过自定义 `useChat` Hook 封装
- **Artifact Export UI**：笔记本页面包含 Export 菜单，支持 Markdown、思维导图、Word、PPT 和 PDF。导出流程为配置要求 → 生成可编辑大纲 → 异步渲染 → 轮询任务状态 → 下载文件。
- **API 客户端**：统一封装在 `lib/utils.ts` 中，包含：
  - `apiFetch()`：通用请求函数（带 JWT 认证）
  - `streamChat()`：SSE 流式聊天
  - `API_ENDPOINTS`：API 路径常量

### 聊天系统

```
用户输入 → useChat.sendMessage()
  → 创建 AbortController（支持取消）
  → 调用 streamChat() 建立 SSE 连接
  → onChunk: 逐字更新 currentContent（流式显示）
  → onDone: 生成完整 ChatMessage 加入消息列表
  → onError: 错误处理（含会话失效检测）
```

**核心特性：**
- SSE 流式输出，实时显示 AI 回复
- 来源引用点击 → 自动切换到 PDF 视图并高亮对应区域
- 会话切换时自动加载历史消息
- 支持消息重新生成（Regenerate）

### PDF 预览与来源定位

```
AI 回复中的 Source Badge 点击
  → setMainViewToPdf(docId, highlightTarget)
  → 主视图切换为 PDF 模式
  → PDF 组件根据 boundingBox 渲染高亮层
  → 用户可直观看到回答对应的原文位置
```

---

## 功能模块

### 1. 认证模块 (`(auth)/`)
- 注册 / 登录页面
- JWT Token 存储在 localStorage
- API 请求自动携带 Authorization Header

### 2. 笔记本工作台 (`notebooks/[id]/`)
三栏布局：

| 区域 | 功能 |
|------|------|
| 左侧边栏 | 笔记本列表、文档列表、文档选择（多选） |
| 中间主区 | 聊天面板 / PDF 预览器 / 文档指南（可切换） |
| 右侧面板 | 研究笔记管理 |

### 3. 智能问答
- SSE 流式输出 AI 回答
- Markdown 渲染（支持 GFM 表格、列表）
- 推荐问题快捷入口
- 来源引用 Badge（可点击跳转到 PDF 对应位置）

### 4. PDF 预览
- 基于 react-pdf 的 PDF 渲染
- 支持页面缩放、翻页
- 来源高亮显示（boundingBox 定位）

### 5. 研究笔记
- 创建 / 编辑 / 删除笔记
- 标签分类管理
- 钉住重要笔记

### 6. 视觉问答 (VQA) — 后端已支持，前端待集成

> 后端 VQA 接口已就绪 (`/api/v1/vqa/image`, `/vqa/image-url`, `/vqa/image-context`)，前端 UI 和 Hook 尚未实现。如需使用，可通过 API 直接调用。

- 图片上传问答
- 图片 URL 问答
- 结合文档上下文的图文增强问答

---

## 快速开始

### 1. 安装依赖

```powershell
cd web
npm install
```

### 2. 配置环境变量

```powershell
Copy-Item .env.example .env.local
```

编辑 `.env.local`：

```env
# 后端 API 地址
NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1
```

### 3. 启动开发服务器

```powershell
npm run dev
```

访问 http://localhost:3000

### 4. 构建生产版本

```powershell
npm run build
npm start
```

---

## 环境变量

| 变量名 | 说明 | 必填 | 默认值 |
|--------|------|------|--------|
| `NEXT_PUBLIC_API_URL` | 后端 API 基础路径 | 是 | - |

---

## 开发规范

### 代码风格
- 使用 TypeScript strict 模式
- 组件采用函数式组件 + Hooks
- UI 组件遵循 shadcn/ui 风格（基于 Radix UI + CVA）

### API 调用规范
- 所有 API 调用通过 `lib/utils.ts` 中的统一方法
- 认证 Token 从 localStorage 读取
- SSE 流式接口使用 `useChat` Hook 封装

### 状态管理规范
- 全局状态使用 Zustand Store
- 组件内部状态使用 useState/useReducer
- 服务端状态使用 SWR（数据获取和缓存）
