'use client';

import { useEffect } from 'react';
import { Button } from '@/components/ui/button';

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

export default function AppError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error('[AppError]', error);

    if (!isRecoverableClientLoadError(error)) return;
    const key = 'notebookmind:lastChunkRecovery';
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
    <div className="h-screen w-screen flex items-center justify-center bg-background p-6">
      <div className="w-full max-w-sm text-center space-y-4">
        <div className="mx-auto h-12 w-12 rounded-full bg-muted flex items-center justify-center text-xl">
          !
        </div>
        <div className="space-y-2">
          <h1 className="text-xl font-semibold">页面加载失败</h1>
          <p className="text-sm text-muted-foreground">
            页面资源可能已更新，清理本地缓存后刷新即可继续。
          </p>
        </div>
        <div className="flex justify-center gap-2">
          <Button onClick={hardReload}>清缓存并刷新</Button>
          <Button variant="outline" onClick={reset}>
            重试
          </Button>
        </div>
      </div>
    </div>
  );
}
