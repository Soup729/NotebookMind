'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function Home() {
  const router = useRouter();

  useEffect(() => {
    // 临时重定向到根目录，实际项目应该有笔记本列表页
    router.replace('/notebooks');
  }, [router]);

  return null;
}
