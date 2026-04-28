'use client';

import { useEffect } from 'react';

function isRecoverableClientLoadError(error: Error & { digest?: string }) {
  const message = `${error.name || ''} ${error.message || ''}`.toLowerCase();
  return (
    message.includes('chunk') ||
    message.includes('loading css') ||
    message.includes('failed to fetch dynamically imported module') ||
    message.includes('module script') ||
    message.includes('load failed')
  );
}

async function clearRuntimeCaches() {
  if (typeof window === 'undefined') return;
  if ('caches' in window) {
    const keys = await caches.keys();
    await Promise.all(keys.map((key) => caches.delete(key)));
  }
}

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error('[GlobalError]', error);

    if (!isRecoverableClientLoadError(error)) return;
    const key = 'notebookmind:lastGlobalChunkRecovery';
    const lastRecovery = Number(sessionStorage.getItem(key) || '0');
    if (Date.now() - lastRecovery < 30_000) return;

    sessionStorage.setItem(key, String(Date.now()));
    clearRuntimeCaches().finally(() => {
      window.location.reload();
    });
  }, [error]);

  const hardReload = async () => {
    await clearRuntimeCaches();
    window.location.reload();
  };

  return (
    <html lang="zh-CN">
      <body>
        <div
          style={{
            minHeight: '100vh',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontFamily:
              'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
            padding: 24,
          }}
        >
          <div style={{ maxWidth: 360, textAlign: 'center' }}>
            <h1 style={{ fontSize: 22, marginBottom: 8 }}>页面加载失败</h1>
            <p style={{ color: '#666', lineHeight: 1.6, marginBottom: 18 }}>
              页面资源可能已更新，清理本地缓存后刷新即可继续。
            </p>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'center' }}>
              <button
                onClick={hardReload}
                style={{
                  height: 34,
                  padding: '0 12px',
                  borderRadius: 6,
                  border: '1px solid #111',
                  background: '#111',
                  color: '#fff',
                  cursor: 'pointer',
                }}
              >
                清缓存并刷新
              </button>
              <button
                onClick={reset}
                style={{
                  height: 34,
                  padding: '0 12px',
                  borderRadius: 6,
                  border: '1px solid #ddd',
                  background: '#fff',
                  color: '#111',
                  cursor: 'pointer',
                }}
              >
                重试
              </button>
            </div>
          </div>
        </div>
      </body>
    </html>
  );
}
