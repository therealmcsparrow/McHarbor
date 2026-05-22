// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { IconChevronRight } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';

export type ContextMenuItem = {
  key: string;
  label: string;
  icon?: React.ReactNode;
  shortcut?: string;
  danger?: boolean;
  disabled?: boolean;
  separator?: boolean;
  children?: ContextMenuItem[];
};

type WorkflowContextMenuProps = {
  x: number;
  y: number;
  items: ContextMenuItem[];
  onSelect: (key: string) => void;
  onClose: () => void;
};

export function WorkflowContextMenu({ x, y, items, onSelect, onClose }: WorkflowContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [openSub, setOpenSub] = useState<string | null>(null);

  useEffect(() => {
    const onMouseDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const onEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('mousedown', onMouseDown, true);
    window.addEventListener('keydown', onEscape);
    return () => {
      window.removeEventListener('mousedown', onMouseDown, true);
      window.removeEventListener('keydown', onEscape);
    };
  }, [onClose]);

  return (
    <div
      ref={menuRef}
      className="fixed z-[100] min-w-[180px] rounded-lg border border-border bg-popover py-1 shadow-xl"
      style={{ left: x, top: y }}
    >
      {items.map((item, i) => {
        if (item.separator) {
          return <div key={`sep-${i}`} className="my-1 border-t border-border" />;
        }

        if (item.children && item.children.length > 0) {
          return (
            <div
              key={item.key}
              className="relative"
              onMouseEnter={() => setOpenSub(item.key)}
              onMouseLeave={() => setOpenSub(null)}
            >
              <Button
                variant="ghost"
                className={cn(
                  'flex w-full items-center justify-start gap-2 rounded-none px-3 py-1.5 text-xs h-auto text-foreground',
                )}
              >
                {item.icon && <span className="size-3.5 shrink-0">{item.icon}</span>}
                <span className="flex-1 text-left">{item.label}</span>
                <IconChevronRight className="size-3 text-muted-foreground" />
              </Button>
              {openSub === item.key && (
                <div
                  className="absolute left-full top-0 z-[101] min-w-[180px] rounded-lg border border-border bg-popover py-1 shadow-xl"
                  style={{ marginLeft: 2 }}
                >
                  {item.children.map((child, ci) => {
                    if (child.separator) {
                      return <div key={`csep-${ci}`} className="my-1 border-t border-border" />;
                    }
                    return (
                      <Button
                        key={child.key}
                        variant="ghost"
                        disabled={child.disabled}
                        onClick={() => {
                          if (!child.disabled) {
                            onSelect(child.key);
                            onClose();
                          }
                        }}
                        className={cn(
                          'flex w-full items-center justify-start gap-2 rounded-none px-3 py-1.5 text-xs h-auto',
                          child.disabled && 'opacity-30 cursor-not-allowed',
                          child.danger
                            ? 'text-red-400 hover:bg-red-500/10'
                            : 'text-foreground',
                        )}
                      >
                        {child.icon && <span className="size-3.5 shrink-0">{child.icon}</span>}
                        <span className="flex-1 text-left">{child.label}</span>
                        {child.shortcut && (
                          <span className="text-[10px] text-muted-foreground/60">{child.shortcut}</span>
                        )}
                      </Button>
                    );
                  })}
                </div>
              )}
            </div>
          );
        }

        return (
          <Button
            key={item.key}
            variant="ghost"
            disabled={item.disabled}
            onClick={() => {
              if (!item.disabled) {
                onSelect(item.key);
                onClose();
              }
            }}
            className={cn(
              'flex w-full items-center justify-start gap-2 rounded-none px-3 py-1.5 text-xs h-auto',
              item.disabled && 'opacity-30 cursor-not-allowed',
              item.danger
                ? 'text-red-400 hover:bg-red-500/10'
                : 'text-foreground',
            )}
          >
            {item.icon && <span className="size-3.5 shrink-0">{item.icon}</span>}
            <span className="flex-1 text-left">{item.label}</span>
            {item.shortcut && (
              <span className="text-[10px] text-muted-foreground/60">{item.shortcut}</span>
            )}
          </Button>
        );
      })}
    </div>
  );
}
