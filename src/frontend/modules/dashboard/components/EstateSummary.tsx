// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useEstateSummary } from '../hooks/useEstateSummary';

type Segment = {
  key: string;
  text: string;
  tone?: 'warning';
};

export function EstateSummary() {
  const { t } = useTranslation('dashboard');
  const { data, isLoading } = useEstateSummary();

  if (isLoading || !data) {
    return <div className="h-5 w-72 animate-pulse rounded bg-muted" aria-hidden="true" />;
  }

  const segments: Segment[] = [{ key: 'environments', text: t('summary.environments', { count: data.environments }) }];

  if (data.containers !== undefined) {
    const containers = t('summary.containers', { count: data.containers });
    segments.push({
      key: 'containers',
      text:
        data.stacks !== undefined ? `${containers} ${t('summary.acrossStacks', { count: data.stacks })}` : containers,
    });
  }

  if (data.vms) {
    const vms = t('summary.vms', { count: data.vms });
    segments.push({
      key: 'vms',
      text: data.vmEnvironments ? `${vms} ${t('summary.acrossEnvironments', { count: data.vmEnvironments })}` : vms,
    });
  }

  if (data.agentsBehind) {
    segments.push({
      key: 'agents',
      text: t('summary.agentsBehind', { count: data.agentsBehind }),
      tone: 'warning',
    });
  }

  if (data.scansNeedReview) {
    segments.push({
      key: 'scans',
      text: t('summary.scansNeedReview', { count: data.scansNeedReview }),
      tone: 'warning',
    });
  }

  if (data.unreachable) {
    segments.push({
      key: 'unreachable',
      text: t('summary.unreachable', { count: data.unreachable }),
      tone: 'warning',
    });
  }

  return (
    <p className="max-w-[80ch] text-sm text-muted-foreground">
      {segments.map((segment, index) => (
        <span key={segment.key}>
          {index > 0 && <span className="px-2 text-border">&middot;</span>}
          <span className={segment.tone === 'warning' ? 'text-warning' : undefined}>{segment.text}</span>
        </span>
      ))}
    </p>
  );
}
