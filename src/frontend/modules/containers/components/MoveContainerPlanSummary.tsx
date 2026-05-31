// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck, IconCopy, IconNetwork, IconStack2 } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import type { MoveContainerPlan } from '../hooks/useContainers';

type MoveContainerPlanSummaryProps = {
  plan: MoveContainerPlan;
  fallbackImage: string;
};

function Section({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="space-y-2 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        {icon}
        {title}
      </div>
      {children}
    </div>
  );
}

export function MoveContainerPlanSummary({ plan, fallbackImage }: MoveContainerPlanSummaryProps) {
  const { t } = useTranslation('containers');

  return (
    <>
      <Section title={t('moveDialog.requiredChanges')} icon={<IconCheck className="size-4 text-emerald-400" />}>
        <ul className="space-y-1 text-sm text-muted-foreground">
          {plan.requiredChanges.map((change) => <li key={change}>{change}</li>)}
        </ul>
      </Section>

      <div className="grid gap-3 md:grid-cols-2">
        <Section title={t('moveDialog.image')} icon={<IconCopy className="size-4 text-blue-400" />}>
          <div className="space-y-2 text-sm">
            <div className="flex items-center justify-between gap-3">
              <span className="truncate font-mono">{plan.image.reference || fallbackImage}</span>
              <Badge variant={plan.image.willTransfer ? 'warning' : 'success'}>
                {plan.image.willTransfer ? t('moveDialog.transfer') : t('moveDialog.available')}
              </Badge>
            </div>
            {plan.image.size ? (
              <div className="text-xs text-muted-foreground">
                {t('moveDialog.imageSize')}: {formatBytes(plan.image.size)}
              </div>
            ) : null}
          </div>
        </Section>

        <Section title={t('moveDialog.stack')} icon={<IconStack2 className="size-4 text-violet-400" />}>
          <div className="text-sm text-muted-foreground">
            {plan.stack.name ? t('moveDialog.stackPreserved', { name: plan.stack.name }) : t('moveDialog.noStack')}
          </div>
        </Section>
      </div>

      <Section title={t('moveDialog.networkSettings')} icon={<IconNetwork className="size-4 text-cyan-400" />}>
        <div className="grid gap-2 md:grid-cols-2">
          {plan.networks.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('moveDialog.noNetworks')}</p>
          ) : plan.networks.map((network) => (
            <div key={network.sourceName || network.name} className="rounded-md border border-border bg-background/50 p-2 text-xs">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">{network.targetName || network.name}</span>
                <Badge variant={network.willCreate ? 'warning' : 'secondary'}>
                  {network.willCreate ? t('moveDialog.create') : t('moveDialog.keep')}
                </Badge>
              </div>
              <div className="mt-1 text-muted-foreground">
                {network.driver ?? '-'} {network.targetIpAddress || network.ipAddress ? `- ${network.targetIpAddress || network.ipAddress}` : ''}
              </div>
            </div>
          ))}
        </div>
      </Section>

      <Section title={t('moveDialog.volumes')} icon={<IconCopy className="size-4 text-lime-400" />}>
        <div className="space-y-2">
          {plan.volumes.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('moveDialog.noVolumes')}</p>
          ) : plan.volumes.map((volume) => (
            <div key={`${volume.type}-${volume.destination}`} className="flex items-center justify-between gap-3 rounded-md border border-border bg-background/50 p-2 text-xs">
              <div className="min-w-0">
                <div className="truncate font-medium">{volume.name || volume.source || volume.destination}</div>
                <div className="truncate text-muted-foreground">{volume.destination}</div>
              </div>
              <Badge variant={volume.manual ? 'warning' : volume.willCreate ? 'secondary' : 'success'}>
                {volume.manual ? t('moveDialog.manual') : volume.willCreate ? t('moveDialog.create') : t('moveDialog.copy')}
              </Badge>
            </div>
          ))}
        </div>
      </Section>

      {plan.warnings.length > 0 && (
        <Section title={t('moveDialog.warnings')} icon={<IconAlertTriangle className="size-4 text-amber-400" />}>
          <ul className="space-y-1 text-sm text-amber-200">
            {plan.warnings.map((warning) => <li key={warning}>{warning}</li>)}
          </ul>
        </Section>
      )}
    </>
  );
}
