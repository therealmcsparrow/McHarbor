// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import type { TablerIcon } from '@tabler/icons-react';
import { Label } from '@resources/components/ui/Label';
import { cn } from '@resources/utils/cn';

type SettingsGroupProps = {
  icon: TablerIcon;
  title: string;
  description?: string;
  /** Small pill next to the title stating who the setting affects. */
  scope?: string;
  children: ReactNode;
  /** Right-aligned footer actions — typically the group's single Save button. */
  actions?: ReactNode;
  className?: string;
};

/**
 * SettingsGroup - one self-contained block of related settings inside a
 * Settings tab. The tab body already sits on a `mc-glass` surface, so the
 * group uses a plain bordered panel (same material as
 * `SelfUpdateSettingsCard`) instead of a nested Card.
 *
 * Each group owns exactly one save flow: its fields go in `children`, its
 * Save button goes in `actions`, so a reader never has to guess which
 * button writes which field.
 */
export function SettingsGroup({
  icon: Icon,
  title,
  description,
  scope,
  children,
  actions,
  className,
}: SettingsGroupProps) {
  return (
    <section className={cn('rounded-lg border border-border p-5', className)}>
      <header className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold text-foreground">{title}</h3>
            {scope && <span className="bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">{scope}</span>}
          </div>
          {description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}
        </div>
      </header>

      <div className="space-y-5">{children}</div>

      {actions && (
        <div className="mt-5 flex flex-wrap items-center justify-end gap-2 border-t border-border pt-4">{actions}</div>
      )}
    </section>
  );
}

type SettingsFieldProps = {
  label: string;
  description?: string;
  /** Secondary line rendered under the control (runtime values, units, warnings). */
  hint?: ReactNode;
  htmlFor?: string;
  children: ReactNode;
  className?: string;
};

/** SettingsField - label + optional description above a single control. */
export function SettingsField({ label, description, hint, htmlFor, children, className }: SettingsFieldProps) {
  return (
    <div className={cn('min-w-0', className)}>
      <Label className="mb-1" htmlFor={htmlFor}>
        {label}
      </Label>
      {description && <p className="mb-2 text-xs text-muted-foreground">{description}</p>}
      {children}
      {hint && <p className="mt-1.5 text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
