// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';

export type DetailTab<T extends string> = {
  id: T;
  label: string;
  /** A count or state dot rendered after the label. */
  adornment?: React.ReactNode;
};

type DetailTabsProps<T extends string> = {
  tabs: DetailTab<T>[];
  value: T;
  onChange: (next: T) => void;
  ariaLabel: string;
};

/**
 * The tab rail that sits *inside* a detail page's hero card.
 *
 * Same shape the Kubernetes page's strip uses, kept here rather than imported
 * from that module so the Swarm pages do not depend on Kubernetes internals
 * for their chrome.
 */
export function DetailTabs<T extends string>({ tabs, value, onChange, ariaLabel }: DetailTabsProps<T>) {
  return (
    <nav className="flex w-fit flex-wrap gap-x-1 rounded-lg bg-muted p-1" aria-label={ariaLabel}>
      {tabs.map((tab) => (
        <Button
          key={tab.id}
          variant="ghost"
          onClick={() => onChange(tab.id)}
          aria-current={value === tab.id ? 'page' : undefined}
          className={cn(
            'inline-flex items-center gap-x-2 rounded-lg px-4 py-2 text-sm font-medium',
            value === tab.id
              ? 'bg-card text-foreground shadow-sm'
              : 'bg-transparent text-muted-foreground hover:text-primary',
          )}
        >
          {tab.label}
          {tab.adornment}
        </Button>
      ))}
    </nav>
  );
}

/** A label/value row, the unit every detail panel is built from. */
export function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[minmax(0,10rem)_1fr] gap-4 py-2">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-sm text-foreground">{children}</dd>
    </div>
  );
}

/** A panel of DetailRows under a heading. */
export function DetailPanel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-border p-5">
      <h2 className="mb-2 text-sm font-semibold text-foreground">{title}</h2>
      <dl className="divide-y divide-border">{children}</dl>
    </section>
  );
}

/** Renders a key/value map, or a dash when there is nothing in it. */
export function KeyValueList({ entries }: { entries: Record<string, string> }) {
  const keys = Object.keys(entries).sort();
  if (keys.length === 0) return <span className="text-muted-foreground">—</span>;

  return (
    <ul className="space-y-1">
      {keys.map((key) => (
        <li key={key} className="font-mono text-xs">
          <span className="text-muted-foreground">{key}</span>
          <span className="text-muted-foreground">=</span>
          <span className="text-foreground">{entries[key]}</span>
        </li>
      ))}
    </ul>
  );
}

/** Renders a string list, or a dash when it is empty. */
export function StringList({ items }: { items: string[] | null | undefined }) {
  if (!items || items.length === 0) return <span className="text-muted-foreground">—</span>;

  return (
    <ul className="space-y-1">
      {items.map((item) => (
        <li key={item} className="font-mono text-xs break-all text-foreground">
          {item}
        </li>
      ))}
    </ul>
  );
}
