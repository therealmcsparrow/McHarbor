// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { DetailPanel, DetailRow } from '../components/DetailTabs';
import { formatBytes, formatCpus, shortId } from '../format';
import type { SwarmNode } from '../types';

export function NodeOverview({ node }: { node: SwarmNode }) {
  const { t } = useTranslation('swarm');

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <DetailPanel title={t('detail.panels.identity')}>
        <DetailRow label={t('columns.id')}>
          <code className="font-mono text-xs">{shortId(node.id)}</code>
        </DetailRow>
        <DetailRow label={t('columns.hostname')}>{node.hostname || '—'}</DetailRow>
        <DetailRow label={t('detail.fields.address')}>{node.addr || '—'}</DetailRow>
        <DetailRow label={t('columns.role')}>{t(`nodeRole.${node.role}`)}</DetailRow>
        <DetailRow label={t('detail.fields.reachability')}>
          {node.reachability || <span className="text-muted-foreground">—</span>}
        </DetailRow>
        <DetailRow label={t('detail.fields.statusMessage')}>
          {node.message || <span className="text-muted-foreground">—</span>}
        </DetailRow>
      </DetailPanel>

      <DetailPanel title={t('detail.panels.host')}>
        <DetailRow label={t('columns.engine')}>{node.engineVersion || '—'}</DetailRow>
        <DetailRow label={t('detail.fields.platform')}>
          {node.os || '—'} / {node.architecture || '—'}
        </DetailRow>
        <DetailRow label={t('detail.fields.cpus')}>{formatCpus(node.nanoCpus)}</DetailRow>
        <DetailRow label={t('detail.fields.memory')}>{formatBytes(node.memoryBytes)}</DetailRow>
        <DetailRow label={t('columns.tasks')}>{node.taskCount}</DetailRow>
      </DetailPanel>
    </div>
  );
}
