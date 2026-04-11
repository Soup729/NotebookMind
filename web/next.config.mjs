/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,

  // ============================================================
  // Turbopack — Next.js 16 默认启用，webpack 配置仍用于生产构建
  // ============================================================
  turbopack: {},

  // ============================================================
  // Server Components 外部包（解决 SSR 兼容性）
  // ============================================================
  serverExternalPackages: [
    'pdfjs-dist',
    'pdfjs-dist/build/pdf',
    'pdfjs-dist/build/pdf.worker',
  ],

  // 编译优化
  compiler: {
    removeConsole: process.env.NODE_ENV === 'production',
  },

  // 开发服务器优化
  devIndicators: {
    buildActivity: false,
  },

  // ============================================================
  // Webpack（仅生产构建 + fallback）
  // ============================================================
  webpack: (config, { dev }) => {
    if (dev) {
      // 网络驱动器启用 polling 模式（更可靠但略耗 CPU）
      const isNetworkDrive = process.cwd().startsWith('F') || process.cwd().startsWith('f');
      config.watchOptions = {
        ...config.watchOptions,
        ignored: ['**/node_modules/**', '**/.next/**'],
        aggregateTimeout: 300,
        poll: isNetworkDrive ? 1000 : false,
      };
    }

    // 生产构建优化
    if (!dev) {
      config.optimization.splitChunks = {
        ...config.optimization.splitChunks,
        chunks: 'all',
        cacheGroups: {
          vendor: {
            test: /[\\/]node_modules[\\/]/,
            name: 'vendors',
            priority: 10,
            reuseExistingChunk: true,
          },
          pdf: {
            test: /[\\/]node_modules[\\/](pdfjs-dist|react-pdf)[\\/]/,
            name: 'pdf',
            priority: 20,
            reuseExistingChunk: true,
          },
        },
      };
    }

    // pdfjs-dist SSR 兼容（Turbopack 不读取此配置）
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
