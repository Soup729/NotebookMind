import Link from "next/link";

export default function HomePage() {
  return (
    <main className="mx-auto flex min-h-screen max-w-6xl flex-col justify-center px-6 py-16">
      <div className="glass-panel rounded-[2rem] border border-line p-10 shadow-panel">
        <div className="mb-10 flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="text-sm font-semibold uppercase tracking-[0.3em] text-accent">Enterprise PDF AI</p>
            <h1 className="mt-3 max-w-3xl text-5xl font-bold leading-tight text-ink">
              把多文档问答从 Demo，真正做成企业可交付的 SaaS 工作台。
            </h1>
          </div>
          <div className="rounded-full border border-ink/10 bg-white/70 px-4 py-2 text-sm text-ink/70">
            Go API + Next.js + Asynq + RAG
          </div>
        </div>

        <div className="grid gap-6 md:grid-cols-3">
          <section className="mesh-card rounded-[1.5rem] border border-line p-6">
            <h2 className="text-xl font-bold">异步文档处理</h2>
            <p className="mt-3 text-sm leading-6 text-ink/70">
              上传即入队，前端轮询任务状态，避免大 PDF 解析阻塞用户操作。
            </p>
          </section>
          <section className="mesh-card rounded-[1.5rem] border border-line p-6">
            <h2 className="text-xl font-bold">来源可追溯问答</h2>
            <p className="mt-3 text-sm leading-6 text-ink/70">
              每条回答都附带检索到的文档片段，方便审阅与二次确认。
            </p>
          </section>
          <section className="mesh-card rounded-[1.5rem] border border-line p-6">
            <h2 className="text-xl font-bold">可视化运营面板</h2>
            <p className="mt-3 text-sm leading-6 text-ink/70">
              仪表盘、搜索、用量统计、文档管理和会话工作台一体化交付。
            </p>
          </section>
        </div>

        <div className="mt-10 flex flex-wrap gap-4">
          <Link
            href="/login"
            className="rounded-full bg-ink px-6 py-3 text-sm font-semibold text-white transition hover:translate-y-[-1px]"
          >
            登录工作台
          </Link>
          <Link
            href="/register"
            className="rounded-full border border-ink px-6 py-3 text-sm font-semibold text-ink transition hover:bg-ink hover:text-white"
          >
            创建账号
          </Link>
        </div>
      </div>
    </main>
  );
}
