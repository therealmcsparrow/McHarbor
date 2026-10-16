// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Text as UiText } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { PermissionRow, WildcardRow, groupByCategory } from './PermissionEditor.parts';
import { cn } from '@resources/utils/cn';
import { usePermissionCatalog, type PermissionCatalogEntry } from '@core/auth/usePermission';

/**
 * PermissionEditor is the picker used by the role editor. It shows
 * every entry in the canonical catalog, grouped by category, with a
 * tri-state checkbox for each permission:
 *
 *   - "force-add" = role grants this permission
 *   - "force-deny" = role explicitly denies it
 *   - "inherit"   = no explicit entry, falls through to wildcard
 *
 * The component is controlled — the parent supplies `value` and an
 * `onChange` that returns the new selection map.
 */
export type PermissionSelection = {
  /** Map of permission key → "grant" or "deny". Missing keys mean "inherit". */
  overrides: Record<string, 'grant' | 'deny'>;
  /** Whether the role has the wildcard. */
  wildcard: boolean;
};

type PermissionEditorProps = {
  value: PermissionSelection;
  onChange: (next: PermissionSelection) => void;
  /** Optional filter (search box contents). Uncontrolled if omitted. */
  searchPlaceholder?: string;
  className?: string;
  /**
   * Overrides the scrolling catalog's height. The default suits the role
   * dialog; a host with more vertical room to spare (the API key form,
   * whose other pickers are single-row) can raise it.
   */
  listClassName?: string;
  readOnly?: boolean;
  selectOnly?: boolean;
  selectLabels?: SelectLabels;
};

export type SelectLabels = { include: string; includeAll: string; clear: string };

export function PermissionEditor({
  value,
  onChange,
  searchPlaceholder,
  className,
  listClassName,
  readOnly = false,
  selectOnly = false,
  selectLabels,
}: PermissionEditorProps) {
  const { t } = useTranslation('security');
  const { data: catalog } = usePermissionCatalog();
  const [filter, setFilter] = useState('');

  const grouped = useMemo(() => groupByCategory(catalog ?? [], filter), [catalog, filter]);

  return (
    <div className={cn('flex h-full flex-col gap-3', className)}>
      <Flex align="center" gap={2}>
        <IconSearch className="size-4 text-muted-foreground" />
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={searchPlaceholder ?? t('permissions.searchPlaceholder')}
          aria-label={t('permissions.searchAriaLabel')}
        />
      </Flex>

      {!selectOnly && (
        <WildcardRow
          active={value.wildcard}
          onToggle={() => {
            if (readOnly) return;
            onChange({ ...value, wildcard: !value.wildcard });
          }}
          disabled={readOnly}
        />
      )}

      <Box className={cn('max-h-[420px] overflow-y-auto rounded-lg border border-border', listClassName)}>
        {grouped.size === 0 ? (
          <Flex align="center" justify="center" className="p-8">
            <UiText size="sm" tone="muted">
              {t('permissions.noMatches')}
            </UiText>
          </Flex>
        ) : (
          Array.from(grouped.entries()).map(([category, entries]) => (
            <CategoryBlock
              key={category}
              category={category}
              entries={entries}
              value={value}
              onChange={onChange}
              disabled={readOnly}
              selectLabels={selectOnly ? selectLabels : undefined}
            />
          ))
        )}
      </Box>
    </div>
  );
}

type CategoryBlockProps = {
  category: string;
  entries: PermissionCatalogEntry[];
  value: PermissionSelection;
  onChange: (next: PermissionSelection) => void;
  disabled?: boolean;
  selectLabels?: SelectLabels;
};

function CategoryBlock({ category, entries, value, onChange, disabled, selectLabels }: CategoryBlockProps) {
  const { t } = useTranslation('security');
  const overrideCount = entries.filter((e) => value.overrides[e.key] !== undefined).length;
  const isAllGranted = entries.every((e) => value.overrides[e.key] === 'grant');
  const isAnyDenied = entries.some((e) => value.overrides[e.key] === 'deny');

  function toggleAll() {
    if (disabled) return;
    const next: Record<string, 'grant' | 'deny'> = { ...value.overrides };
    if (isAllGranted) {
      for (const e of entries) delete next[e.key];
    } else {
      for (const e of entries) next[e.key] = 'grant';
    }
    onChange({ ...value, overrides: next });
  }

  return (
    <Box className="border-b border-border last:border-b-0">
      {/* Opaque, not a tint: this header sticks inside the scrolling list, so a
          translucent background lets the rows scroll through the category label
          and the Grant all button. Same treatment as `DataGridHeader`. */}
      <Flex align="center" justify="between" className="sticky top-0 z-10 bg-muted px-3 py-2">
        <div className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: permission categories come from the backend catalog, including plugin-declared ones */}
          {t(`permissions.${category}`, { defaultValue: category })}
        </div>
        <Flex align="center" gap={2}>
          {overrideCount > 0 && (
            <span className="text-xs text-muted-foreground">
              {t('permissions.overridesCount', { count: overrideCount })}
            </span>
          )}
          <PlainButton
            type="button"
            onClick={toggleAll}
            disabled={disabled}
            className={cn(
              'mc-control rounded-md border px-2 text-xs transition-colors',
              isAllGranted
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-border bg-background text-foreground hover:bg-muted',
              disabled && 'cursor-not-allowed opacity-60',
            )}
            aria-pressed={isAllGranted}
          >
            {selectLabels
              ? isAllGranted
                ? selectLabels.clear
                : selectLabels.includeAll
              : isAllGranted
                ? t('permissions.ungrantAll')
                : t('permissions.grantAll')}
          </PlainButton>
        </Flex>
      </Flex>
      <ul className="divide-y divide-border">
        {entries.map((entry) => (
          <PermissionRow
            key={entry.key}
            entry={entry}
            state={value.overrides[entry.key]}
            onChange={(next) => {
              if (disabled) return;
              const overrides = { ...value.overrides };
              if (next === undefined) {
                delete overrides[entry.key];
              } else {
                overrides[entry.key] = next;
              }
              onChange({ ...value, overrides });
            }}
            disabled={disabled}
            inheritsDeny={isAnyDenied && !value.overrides[entry.key]}
            selectLabels={selectLabels}
          />
        ))}
      </ul>
    </Box>
  );
}
