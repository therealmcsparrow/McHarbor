// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createElement } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { searchIcon } from '@resources/components/globalSearchPlugins';
import { useEnvironmentEstate, type EnvironmentEstateEntry } from '../hooks/useEnvironmentEstate';

const ESTATE_ICON_CLASS = 'size-4 shrink-0';

function EstateTile({ entry }: { entry: EnvironmentEstateEntry }) {
  const { t } = useTranslation('environments');
  // The label is the plugin's own string, in the namespace it declared.
  const label = t(entry.labelKey, { ns: entry.ns });
  const count = entry.running === undefined ? entry.total : `${entry.running}/${entry.total}`;
  const body = (
    <>
      <div className="flex items-center gap-1.5 text-muted-foreground">
        {createElement(searchIcon(entry.icon), { className: ESTATE_ICON_CLASS })}
        <span className="truncate text-[11px]">{label}</span>
      </div>
      <p className="mt-1 text-lg font-semibold text-foreground tabular-nums">{count}</p>
    </>
  );
  const className = 'min-w-0 rounded-lg border border-border/70 bg-background/40 px-3 py-2';
  const title =
    entry.running === undefined ? undefined : t('card.estateRunning', { running: entry.running, total: entry.total });
  return entry.link ? (
    <Link to={entry.link} title={title} className={cn(className, 'transition-colors hover:border-primary/40')}>
      {body}
    </Link>
  ) : (
    <div title={title} className={className}>
      {body}
    </div>
  );
}

/** Tiles for the non-container workloads plugins report on an environment; renders nothing when none do. */
export function EnvironmentEstateRow({ environmentId, enabled }: { environmentId: string; enabled: boolean }) {
  const estate = useEnvironmentEstate(environmentId, enabled);
  if (estate.length === 0) return null;
  return (
    <div className="grid grid-cols-2 gap-2">
      {estate.map((entry) => (
        <EstateTile key={entry.key} entry={entry} />
      ))}
    </div>
  );
}
