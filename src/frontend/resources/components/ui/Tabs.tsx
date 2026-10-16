// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  cloneElement,
  forwardRef,
  isValidElement,
  useEffect,
  useId,
  useState,
  type ComponentPropsWithoutRef,
  type ElementRef,
  type ReactElement,
  type ReactNode,
} from 'react';
import * as TabsPrimitive from '@radix-ui/react-tabs';
import { cn } from '@resources/utils/cn';
import { Button } from './Button';

// Clicks are contained by the menu, so it declares key handling too; its items are real buttons with their own keyboard support.
const ignoreKey = () => undefined;

const HAS_MENU_POPUP = { 'aria-haspopup': 'menu' } as const;

const Tabs = TabsPrimitive.Root;
const tabContextMenuOpenEvent = 'mcharbor:tab-context-menu-open';

const TabsList = forwardRef<ElementRef<typeof TabsPrimitive.List>, ComponentPropsWithoutRef<typeof TabsPrimitive.List>>(
  ({ className, ...props }, ref) => (
    <TabsPrimitive.List
      ref={ref}
      className={cn('inline-flex items-center border-b border-border', className)}
      {...props}
    />
  ),
);
TabsList.displayName = TabsPrimitive.List.displayName;

export type TabContextMenuItem = {
  id: string;
  label: string;
  icon?: ReactNode;
  disabled?: boolean;
  onSelect: () => void;
};

interface TabContextMenuProps {
  items?: readonly TabContextMenuItem[];
  children: ReactElement;
}

function TabContextMenu({ items = [], children }: TabContextMenuProps) {
  const menuId = useId();
  const [isOpen, setOpen] = useState(false);
  const enabledItems = items.filter((item) => !item.disabled);

  useEffect(() => {
    if (!isOpen) return;

    const closeFn = () => setOpen(false);
    const onKeyDown = (evt: KeyboardEvent) => {
      if (evt.key === 'Escape') setOpen(false);
    };

    document.addEventListener('click', closeFn);
    document.addEventListener('contextmenu', closeFn);
    document.addEventListener('keydown', onKeyDown);
    window.addEventListener(tabContextMenuOpenEvent, closeFn);
    return () => {
      document.removeEventListener('click', closeFn);
      document.removeEventListener('contextmenu', closeFn);
      document.removeEventListener('keydown', onKeyDown);
      window.removeEventListener(tabContextMenuOpenEvent, closeFn);
    };
  }, [isOpen]);

  if (enabledItems.length === 0 || !isValidElement(children)) {
    return children;
  }

  return (
    <span
      role="none"
      className="relative inline-flex"
      onContextMenu={(evt) => {
        evt.preventDefault();
        evt.stopPropagation();
        window.dispatchEvent(new Event(tabContextMenuOpenEvent));
        setOpen(true);
      }}
      onKeyDown={(evt) => {
        if ((evt.shiftKey && evt.key === 'F10') || evt.key === 'ContextMenu') {
          evt.preventDefault();
          window.dispatchEvent(new Event(tabContextMenuOpenEvent));
          setOpen(true);
        }
      }}
      data-tab-context-menu="true"
    >
      {cloneElement(children as ReactElement<typeof HAS_MENU_POPUP>, HAS_MENU_POPUP)}
      {isOpen && (
        <span
          id={menuId}
          role="menu"
          tabIndex={-1}
          className="mc-glass-overlay mc-menu absolute top-full left-0 z-50 mt-1 min-w-44 rounded-lg p-1 text-popover-foreground"
          onClick={(evt) => evt.stopPropagation()}
          onKeyDown={ignoreKey}
          onContextMenu={(evt) => evt.stopPropagation()}
        >
          {enabledItems.map((item) => (
            <Button
              key={item.id}
              type="button"
              variant="ghost"
              size="sm"
              role="menuitem"
              className="w-full justify-start gap-2"
              onClick={() => {
                setOpen(false);
                item.onSelect();
              }}
            >
              {item.icon}
              <span className="truncate">{item.label}</span>
            </Button>
          ))}
        </span>
      )}
    </span>
  );
}

type TabsTriggerProps = ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger> & {
  contextMenuItems?: readonly TabContextMenuItem[];
};

const TabsTrigger = forwardRef<ElementRef<typeof TabsPrimitive.Trigger>, TabsTriggerProps>(
  ({ className, contextMenuItems, ...props }, ref) => {
    const trigger = (
      <TabsPrimitive.Trigger
        ref={ref}
        className={cn(
          'inline-flex items-center justify-center gap-1.5 border-b-2 border-transparent px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors',
          'hover:bg-accent hover:text-foreground',
          'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none',
          'disabled:pointer-events-none disabled:opacity-50',
          'data-[state=active]:border-primary data-[state=active]:bg-primary/10 data-[state=active]:font-semibold data-[state=active]:text-primary',
          className,
        )}
        {...props}
      />
    );

    return <TabContextMenu items={contextMenuItems}>{trigger}</TabContextMenu>;
  },
);
TabsTrigger.displayName = TabsPrimitive.Trigger.displayName;

const TabsContent = forwardRef<
  ElementRef<typeof TabsPrimitive.Content>,
  ComponentPropsWithoutRef<typeof TabsPrimitive.Content>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Content
    ref={ref}
    className={cn(
      'mt-4 flex min-h-0 flex-1 flex-col overflow-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none',
      className,
    )}
    {...props}
  />
));
TabsContent.displayName = TabsPrimitive.Content.displayName;

export { Tabs, TabsList, TabsTrigger, TabsContent, TabContextMenu };
