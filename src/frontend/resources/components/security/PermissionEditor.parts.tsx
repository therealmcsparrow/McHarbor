// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { Text as UiText } from '@resources/components/ui/Typography';
import { Flex } from '@resources/components/ui/Layout';
import { Switch } from '@resources/components/ui/Switch';
import { cn } from '@resources/utils/cn';
import type { PermissionCatalogEntry } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import type { SelectLabels } from './PermissionEditor';

type PermissionRowProps = {
  entry: PermissionCatalogEntry;
  state: 'grant' | 'deny' | undefined;
  onChange: (next: 'grant' | 'deny' | undefined) => void;
  disabled?: boolean;
  inheritsDeny?: boolean;
  selectLabels?: SelectLabels;
};

export function PermissionRow({ entry, state, onChange, disabled, inheritsDeny, selectLabels }: PermissionRowProps) {
  const { t } = useTranslation('security');
  return (
    <li className="grid grid-cols-[1fr_auto] items-center gap-3 px-3 py-2">
      <div>
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs text-foreground">{entry.key}</span>
          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] tracking-wide text-muted-foreground uppercase">
            {entry.scope}
          </span>
        </div>
        <UiText size="xs" tone="muted" className="mt-0.5">
          {entry.description}
        </UiText>
        {entry.key === PERM.CONTAINERS_MANAGE && (
          <UiText size="xs" tone="warning" className="mt-0.5">
            {t('permissions.containersManageSelfNote')}
          </UiText>
        )}
      </div>
      <Flex align="center" gap={1}>
        <ToggleButton
          active={state === 'grant'}
          onClick={() => onChange(state === 'grant' ? undefined : 'grant')}
          disabled={disabled}
          label={selectLabels ? selectLabels.include : t('permissions.grant')}
          tone="success"
        />
        {!selectLabels && (
          <ToggleButton
            active={state === 'deny' || (state === undefined && !!inheritsDeny)}
            onClick={() => onChange(state === 'deny' ? undefined : 'deny')}
            disabled={disabled}
            label={t('permissions.deny')}
            tone="destructive"
          />
        )}
      </Flex>
    </li>
  );
}

type ToggleButtonProps = {
  active: boolean;
  onClick: () => void;
  disabled?: boolean;
  label: string;
  tone: 'success' | 'destructive';
};

export function ToggleButton({ active, onClick, disabled, label, tone }: ToggleButtonProps) {
  const baseClasses =
    'mc-control rounded-md border px-2 text-xs transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-ring';
  const toneClasses = {
    success: active
      ? 'border-success bg-success text-success-foreground'
      : 'border-border bg-background text-foreground hover:bg-muted',
    destructive: active
      ? 'border-destructive bg-destructive text-destructive-foreground'
      : 'border-border bg-background text-foreground hover:bg-muted',
  };
  return (
    <PlainButton
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-pressed={active}
      className={cn(baseClasses, toneClasses[tone], disabled && 'cursor-not-allowed opacity-60')}
    >
      {label}
    </PlainButton>
  );
}

export function groupByCategory(
  entries: PermissionCatalogEntry[],
  filter: string,
): Map<string, PermissionCatalogEntry[]> {
  const lc = filter.trim().toLowerCase();
  const filtered = lc
    ? entries.filter(
        (e) =>
          e.key.toLowerCase().includes(lc) ||
          e.category.toLowerCase().includes(lc) ||
          e.description.toLowerCase().includes(lc),
      )
    : entries;
  const out = new Map<string, PermissionCatalogEntry[]>();
  for (const e of filtered) {
    const arr = out.get(e.category) ?? [];
    arr.push(e);
    out.set(e.category, arr);
  }
  // Deterministic category order — alphabetical.
  return new Map([...out.entries()].sort(([a], [b]) => a.localeCompare(b)));
}

type WildcardRowProps = {
  active: boolean;
  onToggle: () => void;
  disabled?: boolean;
};

export function WildcardRow({ active, onToggle, disabled }: WildcardRowProps) {
  const { t } = useTranslation('security');
  const titleId = useId();

  // The row carries a real Switch rather than a button with a decorative
  // square: this is the one control in the editor that grants everything at
  // once, so it should read — to the eye and to a screen reader — as an on/off
  // switch. A native button element may not contain another control, hence the plain
  // container.
  return (
    <Flex
      align="center"
      justify="between"
      gap={3}
      className={cn(
        // Not `mc-control`: that pins the row to the 1.875rem control height,
        // which cannot hold a title plus its description line.
        'rounded-md border px-3 py-2 transition-colors',
        active ? 'border-primary bg-primary/5' : 'border-border bg-card',
        disabled && 'opacity-60',
      )}
    >
      <div>
        <div id={titleId} className="text-sm font-medium text-foreground">
          {t('permissions.wildcardTitle')}
        </div>
        <UiText size="xs" tone="muted">
          {t('permissions.wildcardDescription')}
        </UiText>
      </div>
      <Switch
        checked={active}
        onCheckedChange={onToggle}
        disabled={disabled}
        aria-labelledby={titleId}
        className="shrink-0"
      />
    </Flex>
  );
}
