# NotebookMind Web - 目录结构

> 最后更新：2026-04-12（与实际代码同步）

```
web/
├── .codebuddy/                  # CodeBuddy 配置
├── .env.example                 # 环境变量模板
├── .env.local                   # 本地环境变量（不提交）
├── .next/                       # Next.js 构建缓存
├── DIRECTORY_STRUCTURE.md       # 本文件
├── README.md                    # 项目文档
├── next-env.d.ts                # Next.js 类型声明
├── next.config.mjs              # Next.js 配置（Turbopack + Webpack + SSR fallback）
├── package.json                 # 项目依赖
├── package-lock.json            # 锁定文件
├── postcss.config.mjs           # PostCSS 配置（TailwindCSS + Autoprefixer）
├── src/                         # 源代码
│   ├── app/                     # Next.js App Router 页面
│   │   ├── globals.css          # 全局样式（shadcn/ui CSS 变量体系）
│   │   ├── layout.tsx           # 根布局（Sonner Toaster + 中文 lang）
│   │   ├── page.tsx             # 首页 → 重定向到 /notebooks
│   │   ├── (auth)/              # 认证路由组
│   │   │   └── layout.tsx       # 认证布局
│   │   │       ├── login/
│   │   │       │   └── page.tsx     # 登录页
│   │   │       └── register/
│   │   │           └── page.tsx     # 注册页
│   │   └── notebooks/
│   │       ├── page.tsx             # 笔记本列表页（首页）
│   │       └── [id]/
│   │           └── page.tsx         # 笔记本工作台核心页面（三栏布局）
│   │
│   ├── components/               # 组件库（共 20 个 .tsx）
│   │   ├── chat/                 # 聊天组件（3 个）
│   │   │   ├── ChatPanel.tsx         # 聊天面板主容器（SSE 流式通信）
│   │   │   ├── ChatMessage.tsx       # 单条消息渲染（Markdown + 来源引用）
│   │   │   └── SourceBadge.tsx        # 来源引用 Badge（可点击跳转 PDF）
│   │   ├── guide/                # 文档指南组件（1 个）
│   │   │   └── DocumentGuide.tsx      # 文档摘要/FAQ/关键点展示
│   │   ├── layout/               # 布局组件（2 个）
│   │   │   ├── SourcesPanel.tsx      # 左侧文档来源面板（列表+多选+上传）
│   │   │   └── NotesPanel.tsx         # 右侧研究笔记面板（CRUD+标签+钉住）
│   │   ├── pdf/                  # PDF 组件（3 个）
│   │   │   ├── PdfViewer.tsx           # PDF 查看器主组件（翻页+缩放）
│   │   │   ├── PdfPage.tsx             # 单页 PDF 渲染
│   │   │   └── HighlightLayer.tsx      # 高亮覆盖层（boundingBox 定位）
│   │   └── ui/                   # 基础 UI 组件 - shadcn/ui 风格（11 个）
│   │       ├── avatar.tsx             # 头像
│   │       ├── badge.tsx              # 徽章
│   │       ├── button.tsx             # 按钮（CVA 变体）
│   │       ├── card.tsx               # 卡片
│   │       ├── checkbox.tsx           # 复选框
│   │       ├── dropdown-menu.tsx      # 下拉菜单
│   │       ├── input.tsx              # 输入框
│   │       ├── scroll-area.tsx        # 滚动区域
│   │       ├── skeleton.tsx           # 骨架屏
│   │       ├── textarea.tsx           # 多行输入框
│   │       └── tooltip.tsx            # 提示框
│   │
│   ├── hooks/                     # 自定义 Hooks（3 个）
│   │   ├── useChat.ts                 # SSE 流式聊天 Hook（取消/重新生成/410检测）
│   │   ├── useNotebook.ts             # 笔记本 CRUD + 文档/会话操作（SWR 封装）
│   │   └── useNotes.ts                # 研究笔记管理 CRUD（SWR 封装）
│   │
│   ├── lib/                       # 工具函数库（1 个）
│   │   └── utils.ts                   # 全部工具集：
│   │                                     # cn() / apiFetch() / streamChat()
│   │                                     # API_ENDPOINTS / parseSourceCitations()
│   │                                     # formatDate() / debounce() / generateId()
│   │
│   ├── store/                     # 状态管理（1 个）
│   │   └── useNotebookStore.ts        # Zustand Store（persist + devtools）
│   │                                     # 含 Selector Hooks 和 Action Hooks
│   │                                     # 含高亮工具函数 isHighlightInViewport/calculateHighlightStyle
│   │
│   └── types/                     # TypeScript 类型定义（2 个）
│       ├── api.d.ts                    # API 完整类型（41 个接口/类型）
│       └── pdf.d.ts                    # PDF 特有类型
│
├── tailwind.config.ts            # Tailwind 配置（HSL 主题 + 自定义动画）
├── tsconfig.json                 # TypeScript 配置（ES2022 strict, @/* 路径别名）
└── ...
```

## 文件统计

| 类别 | 数量 | 说明 |
|------|------|------|
| 页面路由 | 5 | `/`, `/login`, `/register`, `/notebooks`, `/notebooks/[id]` |
| 业务组件 | 9 | chat(3) + guide(1) + layout(2) + pdf(3) |
| UI 基础组件 | 11 | shadcn/ui 风格，基于 Radix UI + CVA |
| 自定义 Hooks | 3 | useChat, useNotebook, useNotes |
| Store | 1 | Zustand（持久化 notebookId/sessionId） |
| 类型文件 | 2 | api.d.ts (414 行), pdf.d.ts |
| 工具文件 | 1 | utils.ts（集中所有 API 和通用函数） |

## 关键设计决策

- **无 services 层**：API 调用逻辑集中在 `lib/utils.ts` 的 `apiFetch()` 和 `API_ENDPOINTS` 中，Hooks 直接调用这些方法
- **无 public/ 目录**：当前项目无静态资源需求
- **无 app/api/**：前端不实现 API 代理路由，直接请求后端 `NEXT_PUBLIC_API_URL`
- **VQA 未集成前端**：后端 VQA 接口已就绪 (`/vqa/*`)，前端暂无对应 UI/Hook
- **依赖冗余**：`@radix-ui/react-dialog` 和 `@radix-ui/react-select` 已安装但当前无使用组件
