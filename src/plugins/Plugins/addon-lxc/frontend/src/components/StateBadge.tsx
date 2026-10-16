// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { Container } from '../types';

/**
 * A container's state.
 *
 * Two states get a tooltip because neither means what a reader would assume:
 *
 *   - "unknown" is a real state, not a rendering failure. It means the owning
 *     agent is offline, so nothing about the container is confirmed. The
 *     container is almost certainly still running under systemd — but nothing
 *     can say so, and pretending otherwise is how a management UI lies.
 *   - "frozen" is not stopped. Every process is suspended and every byte of
 *     memory, every open file and every network connection is still there.
 */
export function StateBadge({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const label = t(`state.${container.state}`, container.state);

  if (container.state === 'unknown' || !container.hostAvailable) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="outline">{t('state.unknown')}</Badge>
        </TooltipTrigger>
        <TooltipContent>{t('state.unknownHint')}</TooltipContent>
      </Tooltip>
    );
  }

  if (container.state === 'frozen') {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="warning">{label}</Badge>
        </TooltipTrigger>
        <TooltipContent>{t('state.frozenHint')}</TooltipContent>
      </Tooltip>
    );
  }

  switch (container.state) {
    case 'running':
      return <Badge variant="success">{label}</Badge>;
    case 'crashed':
      return <Badge variant="destructive">{label}</Badge>;
    default:
      return <Badge variant="secondary">{label}</Badge>;
  }
}

/**
 * Whether a container is unprivileged, shown wherever a container is listed.
 *
 * Privileged is the one that gets the loud treatment: root inside a privileged
 * container is root on the host, and that fact should never be something an
 * operator has to open a detail page to discover.
 */
export function IsolationBadge({ container }: { container: Container }) {
  const { t } = useTranslation('ct');

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant={container.privileged ? 'destructive' : 'secondary'}>
          {container.privileged ? t('isolation.privileged') : t('isolation.unprivileged')}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        {container.privileged ? t('isolation.privilegedHint') : t('isolation.unprivilegedHint')}
      </TooltipContent>
    </Tooltip>
  );
}
