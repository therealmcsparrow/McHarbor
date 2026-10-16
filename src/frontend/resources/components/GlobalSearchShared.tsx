// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const GROUP_CLASS = 'px-2 py-1 text-xs font-medium text-muted-foreground';
export const ITEM_CLASS =
  'flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent';

export function EnvTag({ name: itemName }: { name: string }) {
  return (
    <span className="ml-auto shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
      {itemName}
    </span>
  );
}
