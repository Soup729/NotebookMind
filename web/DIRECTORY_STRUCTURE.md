# ============================================================
# NotebookMind - 目录结构
# ============================================================

enterprise-pdf-web/
├── src/
│   ├── app/                          # Next.js App Router
│   │   ├── (auth)/                   # 认证路由组
│   │   │   ├── layout.tsx            # 认证布局
│   │   │   ├── login/page.tsx        # 登录页
│   │   │   └── register/page.tsx    # 注册页
│   │   ├── (dashboard)/              # 仪表盘路由组
│   │   │   ├── layout.tsx           # 仪表盘布局
│   │   │   ├── page.tsx             # 首页/笔记本列表
│   │   │   ├── documents/page.tsx   # 文档管理页
│   │   │   └── usage/page.tsx       # 使用统计页
│   │   ├── notebooks/
│   │   │   └── [id]/
│   │   │       └── page.tsx          # 笔记本工作台 (核心页面)
│   │   ├── layout.tsx                # 根布局
│   │   ├── page.tsx                  # 根页面 (重定向)
│   │   └── globals.css               # 全局样式
│   │
│   ├── components/                   # 组件库
│   │   ├── ui/                       # Shadcn UI 基础组件
│   │   │   ├── button.tsx
│   │   │   ├── card.tsx
│   │   │   ├── checkbox.tsx
│   │   │   ├── dialog.tsx
│   │   │   ├── dropdown-menu.tsx
│   │   │   ├── input.tsx
│   │   │   ├── scroll-area.tsx
│   │   │   ├── select.tsx
│   │   │   ├── skeleton.tsx
│   │   │   ├── textarea.tsx
│   │   │   └── avatar.tsx
│   │   ├── layout/                   # 布局组件
│   │   │   ├── SourcesPanel.tsx      # 左侧文档面板
│   │   │   ├── NotesPanel.tsx        # 右侧笔记面板
│   │   │   └── ChatInterface.tsx     # 底部对话界面
│   │   ├── pdf/                      # PDF 相关组件
│   │   │   ├── PdfViewer.tsx         # PDF 阅读器 + 高亮
│   │   │   ├── PdfPage.tsx           # 单页 PDF 组件
│   │   │   └── HighlightLayer.tsx   # 高亮图层
│   │   ├── chat/                     # 对话相关组件
│   │   │   ├── ChatPanel.tsx         # 流式对话面板
│   │   │   ├── ChatMessage.tsx       # 单条消息
│   │   │   ├── SourceBadge.tsx       # 来源徽章
│   │   │   └── SuggestedQueries.tsx   # 建议问题
│   │   ├── guide/                    # 指南组件
│   │   │   ├── DocumentGuide.tsx     # 文档指南
│   │   │   ├── SummaryCard.tsx       # 摘要卡片
│   │   │   └── FaqCard.tsx           # FAQ 卡片
│   │   └── notes/                    # 笔记组件
│   │       ├── NoteCard.tsx          # 笔记卡片
│   │       └── NoteEditor.tsx         # 笔记编辑器
│   │
│   ├── hooks/                        # 自定义 Hooks
│   │   ├── useNotebook.ts            # 笔记本 SWR Hook
│   │   ├── useChat.ts                # 聊天 SSE Hook
│   │   └── useNotes.ts               # 笔记 SWR Hook
│   │
│   ├── services/                     # API 服务层
│   │   ├── notebook.ts               # 笔记本 API
│   │   ├── chat.ts                  # 聊天 API (SSE)
│   │   ├── document.ts               # 文档 API
│   │   └── note.ts                  # 笔记 API
│   │
│   ├── store/                        # Zustand 状态管理
│   │   └── useNotebookStore.ts       # 笔记本全局状态
│   │
│   ├── types/                        # TypeScript 类型定义
│   │   ├── api.d.ts                  # API 接口定义
│   │   └── pdf.d.ts                  # PDF 相关类型
│   │
│   └── lib/                          # 工具库
│       ├── utils.ts                  # 工具函数 (cn, etc.)
│       ├── api.ts                    # API 基础配置
│       └── constants.ts              # 常量定义
│
├── public/                           # 静态资源
├── .env.local                        # 环境变量
├── package.json
├── tsconfig.json
├── tailwind.config.ts
├── next.config.mjs
└── README.md
