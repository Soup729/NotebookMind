'use client';

import React, { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { Loader2, Plus, FileText, MoreHorizontal, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { formatDate } from '@/lib/utils';
import type { Notebook } from '@/types/api';

export default function NotebooksPage() {
  const router = useRouter();
  const [isLoading, setIsLoading] = useState(true);
  const [notebooks, setNotebooks] = useState<Notebook[]>([]);

  // 检查登录状态
  useEffect(() => {
    const token = localStorage.getItem('auth_token');
    if (!token) {
      router.replace('/login');
      return;
    }
    fetchNotebooks(token);
  }, [router]);

  // 获取笔记本列表
  const fetchNotebooks = async (token: string) => {
    try {
      const response = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/notebooks`,
        {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        }
      );

      if (response.status === 401) {
        localStorage.removeItem('auth_token');
        router.replace('/login');
        return;
      }

      if (!response.ok) {
        throw new Error('获取笔记本列表失败');
      }

      const data = await response.json();
      setNotebooks(data.items || []);
    } catch (error) {
      toast.error('加载笔记本失败');
    } finally {
      setIsLoading(false);
    }
  };

  // 创建新笔记本
  const handleCreateNotebook = async () => {
    const token = localStorage.getItem('auth_token');
    if (!token) {
      router.replace('/login');
      return;
    }

    try {
      const response = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/notebooks`,
        {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${token}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            title: `新笔记本 ${new Date().toLocaleString()}`,
            description: '',
          }),
        }
      );

      if (!response.ok) {
        throw new Error('创建笔记本失败');
      }

      const data = await response.json();
      toast.success('笔记本创建成功');
      // 跳转到新创建的笔记本
      router.push(`/notebooks/${data.notebook.id}`);
    } catch (error) {
      toast.error('创建笔记本失败');
    }
  };

  // 登出
  const handleLogout = () => {
    localStorage.removeItem('auth_token');
    router.replace('/login');
    toast.success('已退出登录');
  };

  // 打开笔记本
  const handleOpenNotebook = (notebookId: string) => {
    router.push(`/notebooks/${notebookId}`);
  };

  // 删除笔记本
  const handleDeleteNotebook = async (e: React.MouseEvent, notebookId: string) => {
    e.stopPropagation();

    if (!confirm('确定要删除这个笔记本吗？删除后无法恢复。')) {
      return;
    }

    const token = localStorage.getItem('auth_token');
    if (!token) {
      router.replace('/login');
      return;
    }

    try {
      const response = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/notebooks/${notebookId}`,
        {
          method: 'DELETE',
          headers: {
            Authorization: `Bearer ${token}`,
          },
        }
      );

      if (!response.ok) {
        throw new Error('删除笔记本失败');
      }

      toast.success('笔记本已删除');
      // 从列表中移除
      setNotebooks((prev) => prev.filter((nb) => nb.id !== notebookId));
    } catch (error) {
      toast.error('删除笔记本失败');
    }
  };

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-slate-50 to-slate-100">
        <div className="container mx-auto px-4 py-8">
          <div className="flex items-center justify-between mb-8">
            <h1 className="text-2xl font-bold">我的笔记本</h1>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {[1, 2, 3].map((i) => (
              <Card key={i}>
                <CardHeader>
                  <Skeleton className="h-5 w-3/4" />
                  <Skeleton className="h-4 w-1/2 mt-2" />
                </CardHeader>
                <CardContent>
                  <Skeleton className="h-4 w-full" />
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-50 to-slate-100">
      <div className="container mx-auto px-4 py-8">
        {/* 头部 */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-2xl font-bold">我的笔记本</h1>
            <p className="text-sm text-muted-foreground mt-1">
              共 {notebooks.length} 个笔记本
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button onClick={handleCreateNotebook}>
              <Plus className="w-4 h-4 mr-2" />
              新建笔记本
            </Button>
            <Button variant="outline" onClick={handleLogout}>
              退出登录
            </Button>
          </div>
        </div>

        {/* 笔记本列表 */}
        {notebooks.length === 0 ? (
          <Card className="p-12 text-center">
            <FileText className="w-12 h-12 mx-auto mb-4 text-muted-foreground" />
            <h3 className="text-lg font-medium mb-2">暂无笔记本</h3>
            <p className="text-sm text-muted-foreground mb-4">
              创建您的第一个笔记本，开始使用 PDF AI
            </p>
            <Button onClick={handleCreateNotebook}>
              <Plus className="w-4 h-4 mr-2" />
              创建笔记本
            </Button>
          </Card>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {notebooks.map((notebook) => (
              <Card
                key={notebook.id}
                className="cursor-pointer hover:shadow-lg transition-shadow"
                onClick={() => handleOpenNotebook(notebook.id)}
              >
                <CardHeader className="pb-2">
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <CardTitle className="truncate">{notebook.title}</CardTitle>
                      <CardDescription className="mt-1">
                        {notebook.document_cnt} 个文档
                      </CardDescription>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 hover:text-destructive hover:bg-destructive/10"
                      onClick={(e) => handleDeleteNotebook(e, notebook.id)}
                      title="删除笔记本"
                    >
                      <Trash2 className="w-4 h-4 text-destructive" />
                    </Button>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-xs text-muted-foreground">
                    创建于 {formatDate(notebook.created_at)}
                  </p>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
