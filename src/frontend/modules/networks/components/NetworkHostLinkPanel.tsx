// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { NetworkInfo } from '@core/types/docker';
import { PERM } from '@core/auth/permissionKeys';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { useApplyHostLink, useNetworkHostLink } from '../hooks/useNetworks';
import { parseHostLinkLabels } from '../utils/hostLinkLabels';

const STATE_VARIANT = { present: 'success', missing: 'warning', unknown: 'secondary', none: 'secondary' } as const;

export function NetworkHostLinkPanel({ network }: { network: NetworkInfo }) {
  const { t } = useTranslation('networks');
  const link = parseHostLinkLabels(network.Labels);
  const hostLinkStatus = useNetworkHostLink(network.Id, link !== null);
  const apply = useApplyHostLink(network.Id);

  if (!link) return null;
  const state = hostLinkStatus.data?.state ?? 'unknown';
  const canApply = hostLinkStatus.data?.available === true;

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('hostLink.title')}
      </h3>
      <InfoRow label={t('hostLink.name')}>
        <span className="font-mono text-xs">{link.name}</span>
      </InfoRow>
      <InfoRow label={t('hostLink.parent')}>
        <span className="font-mono text-xs">{network.Options?.parent ?? '-'}</span>
      </InfoRow>
      {link.ipv4 && (
        <InfoRow label={t('hostLink.ipv4')}>
          <span className="font-mono text-xs">{link.ipv4}</span>
        </InfoRow>
      )}
      {link.ipv6 && (
        <InfoRow label={t('hostLink.ipv6')}>
          <span className="font-mono text-xs">{link.ipv6}</span>
        </InfoRow>
      )}
      {link.ipv6Auto && <InfoRow label={t('hostLink.ipv6')}>{t('hostLink.auto')}</InfoRow>}
      <InfoRow label={t('hostLink.state')}>
        <Badge variant={STATE_VARIANT[state]} className="px-1.5 py-0 text-[10px]">
          {t(`hostLink.states.${state}`)}
        </Badge>
      </InfoRow>
      {hostLinkStatus.isError && (
        <ErrorState variant="inline" error={hostLinkStatus.error} onRetry={() => void hostLinkStatus.refetch()} />
      )}
      {state === 'missing' && <p className="mt-2 text-xs text-muted-foreground">{t('hostLink.missingHint')}</p>}
      {hostLinkStatus.data && !canApply && hostLinkStatus.data.unavailable && (
        <p className="mt-2 text-xs text-muted-foreground">
          {t(`hostLink.unavailable.${hostLinkStatus.data.unavailable}`)}
        </p>
      )}
      <div className="mt-4">
        <Button
          variant="outline"
          size="sm"
          permission={PERM.NETWORKS_MANAGE}
          disabled={!canApply || apply.isPending}
          onClick={() => apply.mutate()}
        >
          {apply.isPending ? t('hostLink.reapplying') : t('hostLink.reapply')}
        </Button>
      </div>
    </div>
  );
}
