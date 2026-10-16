// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBox, IconStack2 } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { SecretApplyStatus, SecretImpactResource } from '../types';

type SecretImpactRowProps = {
  resource: SecretImpactResource;
  version: number;
  selected: boolean;
  onSelectedChange: (selected: boolean) => void;
  status?: SecretApplyStatus;
  disabled: boolean;
};

const STATUS_VARIANT: Record<SecretApplyStatus, 'success' | 'destructive' | 'secondary'> = {
  applied: 'success',
  failed: 'destructive',
  unsupported: 'secondary',
};

export function SecretImpactRow({
  resource,
  version,
  selected,
  onSelectedChange,
  status: statusValue,
  disabled,
}: SecretImpactRowProps) {
  const { t } = useTranslation('secrets');
  const Icon = resource.resourceType === 'stack' ? IconStack2 : IconBox;
  // A plugin took a one-time copy (a VM's cloud-init password, for one), so
  // "a rotation already reaches it" — the generic unsupported hint — is false.
  const copiedOnce = resource.resourceType === 'plugin';
  const selectable = resource.supported && !resource.missing && !disabled;

  return (
    <li className="flex items-start gap-3 rounded-md border border-border p-3">
      <Checkbox
        className="mt-1"
        checked={selected}
        disabled={!selectable}
        onCheckedChange={(next) => onSelectedChange(next === true)}
        aria-label={t('impact.select', { name: resource.resourceName })}
      />

      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <Icon className="size-4 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium">{resource.resourceName}</span>
          <span className="text-xs text-muted-foreground">{t(`impact.type.${resource.resourceType}`)}</span>
        </div>

        {resource.fields.length > 0 && (
          <p className="truncate font-mono text-xs text-muted-foreground">{resource.fields.join(', ')}</p>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-2">
        {resource.missing && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge variant="destructive">{t('impact.missing')}</Badge>
            </TooltipTrigger>
            <TooltipContent>{t('impact.missingHint')}</TooltipContent>
          </Tooltip>
        )}

        {!resource.missing && !resource.supported && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge variant="secondary">{t(copiedOnce ? 'impact.copied' : 'impact.unsupported')}</Badge>
            </TooltipTrigger>
            <TooltipContent>{t(copiedOnce ? 'impact.copiedHint' : 'impact.unsupportedHint')}</TooltipContent>
          </Tooltip>
        )}

        {!resource.missing && resource.supported && resource.stale && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge variant="warning">{t('impact.stale')}</Badge>
            </TooltipTrigger>
            <TooltipContent>
              {resource.appliedVersion > 0
                ? t('impact.staleHint', { applied: resource.appliedVersion, current: version })
                : t('impact.staleUnknownHint')}
            </TooltipContent>
          </Tooltip>
        )}

        {!resource.missing && resource.supported && !resource.stale && (
          <Badge variant="success">{t('impact.current')}</Badge>
        )}

        {statusValue && <Badge variant={STATUS_VARIANT[statusValue]}>{t(`apply.status.${statusValue}`)}</Badge>}
      </div>
    </li>
  );
}
