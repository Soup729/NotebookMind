/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,

  // ============================================================
  // Turbopack 配置（Next.js 16 默认使用 Turbopack）
  // 替代原 webpack 配置，编译速度提升 5-10x
  // ============================================================
  turbopack: {
    // 解析别名 — 替代 webpack resolve.alias
    resolveAlias: {
      // pdfjs-dist 的 Node.js 内置模块在浏览器端的 polyfill
      // 等同于 webpack 的 resolve.fallback: { fs: false, path: false, crypto: false }
    },
  },

  // ============================================================
  // Server Components 外部包配置
  // 解决 pdfjs-dist 等 Node.js 模块在 SSR/客户端的兼容性问题
  // ============================================================
  serverExternalPackages: [
    'pdfjs-dist',
    'pdfjs-dist/build/pdf',
    'pdfjs-dist/build/pdf.worker',
  ],

  // 编译优化
  compiler: {
    // 移除 console.log（生产环境）
    removeConsole: process.env.NODE_ENV === 'production',
  },

  // 开发服务器优化
  devIndicators: {
    buildActivity: false,
  },

  // 缓存优化 - 网络驱动器加速
  ...(process.env.NEXT_DEV_CACHE_DIR ? {
    cacheHandler: process.env.NEXT_DEV_CACHE_DIR,
    cacheMaxMemorySize: 0,
  } : {}),

  // ============================================================
  // Webpack 兼容层（保留用于生产构建和 fallback 场景）
  // 开发环境优先使用 Turbopack，生产构建仍走 webpack
  // ============================================================
  webpack: (config, { dev }) => {
    if (dev) {
      config.watchOptions = {
        ...config.watchOptions,
        ignored: ['**/node_modules/**', '**/.next/**'],
        aggregateTimeout: 300,
        poll: false,
      };
    }

    // 解决 pdfjs-dist 在 SSR 下的兼容性问题
    // Turbopack 不读取此配置，仅 webpack 构建时生效
    config.resolve.fallback = {
      ...config.resolve.fallback,
      fs: false,
      path: false,
      crypto: false,
    };

    return config;
  },
}

export default nextConfig
