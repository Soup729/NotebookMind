// ============================================================
// Tooltip 组件（轻量实现）
// ============================================================

'use client';

import * as React from 'react';
import { cn } from '@/lib/utils';

// ============================================================
// Context & Provider
// ============================================================

interface TooltipContextValue {
  enabled: boolean;
  setEnabled: (v: boolean) => void;
}

const TooltipContext = React.createContext<TooltipContextValue>({
  enabled: false,
  setEnabled: () => {},
});

function useTooltip() {
  return React.useContext(TooltipContext);
}

// ============================================================
// TooltipProvider - 包裹所有 Tooltip
// ============================================================

interface TooltipProviderProps {
  children: React.ReactNode;
  delayDuration?: number;
}

export function TooltipProvider({ children, delayDuration = 200 }: TooltipProviderProps) {
  const [enabled, setEnabled] = React.useState(false);
  const timeoutRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSetEnabled = React.useCallback((v: boolean) => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    if (v) {
      timeoutRef.current = setTimeout(() => setEnabled(true), delayDuration);
    } else {
      setEnabled(false);
    }
  }, [delayDuration]);

  // cleanup on unmount
  React.useEffect(() => () => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
  }, []);

  return (
    <TooltipContext.Provider value={{ enabled, setEnabled: handleSetEnabled }}>
      {children}
    </TooltipContext.Provider>
  );
}

// ============================================================
// Tooltip - 根容器
// ============================================================

interface TooltipProps {
  children: React.ReactNode;
  side?: 'top' | 'bottom' | 'left' | 'right';
  align?: 'start' | 'center' | 'end';
  className?: string;
}

export function Tooltip({ children, side = 'top', align = 'center', className }: TooltipProps) {
  const ctx = useTooltip();
  const [position, setPosition] = React.useState({ top: 0, left: 0 });
  const triggerRef = React.useRef<HTMLDivElement>(null);

  const updatePosition = React.useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();

    let top = 0, left = 0;

    switch (side) {
      case 'top':
        top = rect.top + window.scrollY - 8;
        left = rect.left + window.scrollX + (align === 'center' ? rect.width / 2 : align === 'end' ? rect.width : 0);
        break;
      case 'bottom':
        top = rect.bottom + window.scrollY + 8;
        left = rect.left + window.scrollX + (align === 'center' ? rect.width / 2 : align === 'end' ? rect.width : 0);
        break;
      default:
        top = rect.top + window.scrollY + rect.height / 2;
        left = rect.right + window.scrollX + 8;
    }

    setPosition({ top, left });
  }, [side, align]);

  React.useEffect(() => {
    if (ctx.enabled) updatePosition();
  }, [ctx.enabled, updatePosition]);

  const alignClass = align === 'center' ? '-translate-x-1/2' :
                       align === 'end' ? '-translate-x-full' : '';

  return (
    <div ref={triggerRef} className="inline-flex" onMouseEnter={() => ctx.setEnabled(true)} onMouseLeave={() => ctx.setEnabled(false)}>
      {children}
      {ctx.enabled && (
        <div
          className={cn(
            'fixed z-50 px-3 py-1.5 text-xs rounded-md bg-primary text-primary-foreground shadow-lg pointer-events-none',
            'animate-in fade-in zoom-in-95 duration-150',
            side === 'top' && `bottom-full left-1/2 mb-2 -translate-x-1/2`,
            side === 'bottom' && `top-full left-1/2 mt-2 -translate-x-1/2`,
            className
          )}
          role="tooltip"
        >
          {/* Content will be rendered by TooltipContent */}
        </div>
      )}
    </div>
  );
}

// ============================================================
// TooltipTrigger - 触发元素
// ============================================================

interface TooltipTriggerProps {
  children: React.ReactElement;
  asChild?: boolean;
}

export function TooltipTrigger({ children }: TooltipTriggerProps) {
  // In this simple implementation, the trigger is just rendered inside Tooltip
  // The parent div handles mouse events
  return <>{children}</>;
}

// ============================================================
// TooltipContent - 提示内容
// ============================================================

interface TooltipContentProps {
  children: React.ReactNode;
  side?: 'top' | 'bottom' | 'left' | 'right';
  className?: string;
}

export function TooltipContent({ children, className }: TooltipContentProps) {
  const ctx = useTooltip();

  if (!ctx.enabled) return null;

  return (
    <div
      className={cn(
        'inline-block max-w-xs rounded-md bg-popover border px-3 py-1.5 text-sm text-popover-foreground shadow-md z-50',
        'animate-in fade-in zoom-in-95 duration-150',
        className
      )}
      role="tooltip"
    >
      {children}
    </div>
  );
}
