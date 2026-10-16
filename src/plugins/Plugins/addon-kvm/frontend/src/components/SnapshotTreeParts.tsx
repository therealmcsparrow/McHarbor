// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Fragment } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight, IconDeviceDesktop } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import type { VM } from '../types';

export function ToggleButton({
  expanded,
  name: nameValue,
  onClick,
}: {
  expanded: boolean;
  name: string;
  onClick: () => void;
}) {
  const { t } = useTranslation('vm');
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-6 shrink-0 self-center"
      aria-expanded={expanded}
      aria-label={
        expanded ? t('snapshots.tree.collapse', { name: nameValue }) : t('snapshots.tree.expand', { name: nameValue })
      }
      onClick={onClick}
    >
      {expanded ? (
        <IconChevronDown className="size-4" aria-hidden="true" />
      ) : (
        <IconChevronRight className="size-4" aria-hidden="true" />
      )}
    </Button>
  );
}

export function Rails({ rails, isLast }: { rails: boolean[]; isLast: boolean }) {
  if (rails.length === 0) return null;
  return (
    <div className="flex shrink-0" aria-hidden="true">
      {rails.slice(0, -1).map((continues, depth) => (
        <Fragment key={depth}>
          <span className={`w-6 ${continues ? 'border-l border-border' : ''}`} />
        </Fragment>
      ))}
      <span className="relative w-6">
        <span className={`absolute top-0 left-0 w-px bg-border ${isLast ? 'h-[1.1rem]' : 'h-full'}`} />
        <span className="absolute top-[1.1rem] left-0 h-px w-6 bg-border" />
      </span>
    </div>
  );
}

export function CurrentState({ vm, rails }: { vm: VM; rails: boolean[] }) {
  const { t } = useTranslation('vm');
  return (
    <li>
      <div className="flex items-stretch">
        <Rails rails={rails} isLast />
        <span className="w-7 shrink-0" aria-hidden="true" />
        <div className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-dashed border-primary/60 bg-primary/5 px-3 py-1.5">
          <IconDeviceDesktop className="size-3.5 shrink-0 text-primary" aria-hidden="true" />
          <span className="text-sm font-medium text-foreground">{t('snapshots.tree.now')}</span>
          <span className="truncate text-[11px] text-muted-foreground">{t(`state.${vm.state}`)}</span>
        </div>
      </div>
    </li>
  );
}
