// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Button } from '@resources/components/ui/Button';

type FilterButtonProps = {
  active: boolean;
  onClick: () => void;
  label: string;
  count: number;
  tone?: 'success' | 'default';
};

export function FilterButton({ active, onClick, label, count, tone = 'default' }: FilterButtonProps) {
  const activeTone =
    tone === 'success' && active
      ? 'bg-chart-1/20 text-chart-1'
      : active
        ? 'bg-primary/15 text-primary'
        : 'text-muted-foreground hover:text-foreground';
  return (
    <Button variant={active ? 'default' : 'ghost'} size="sm" onClick={onClick} className={`gap-1.5 ${activeTone}`}>
      <span>{label}</span>
      <span className="bg-background/60 px-1.5 text-[10px] text-muted-foreground tabular-nums">{count}</span>
    </Button>
  );
}
