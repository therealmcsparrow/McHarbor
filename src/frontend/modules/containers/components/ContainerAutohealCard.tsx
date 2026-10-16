// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconHeartbeat } from '@tabler/icons-react';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Subheading, Text as BodyText } from '@resources/components/ui/Typography';
import { Switch } from '@resources/components/ui/Switch';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Badge } from '@resources/components/ui/Badge';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useAutohealPreference, useSetAutohealPreference } from '../hooks/useAutoheal';

type ContainerAutohealCardProps = {
  containerId: string;
};

/**
 * Auto-heal: restart this container automatically when its healthcheck starts
 * reporting unhealthy.
 *
 * The engine in `modules/autoheal/runtime.go` has always read this preference;
 * what was missing was any way to see or set it, which left both autoheal
 * permission keys enforced on routes no screen reached.
 *
 * Every string here already existed in all six locales (`containers.autoheal.*`,
 * `containers.toast.autohealUpdated`) — the copy for this card was written
 * before the card was. Reuse rather than new keys is deliberate: the existing
 * set is fully translated and describes the engine's real behaviour, including
 * the cooldown schedule and the first-heal rule.
 *
 * The gate is **any-of**: the backend takes `containers.manage` OR
 * `autoheal.manage` (`RequireAnyPermission`), so gating on the autoheal key
 * alone would disable the switch for a container manager the backend accepts.
 */
export function ContainerAutohealCard({ containerId }: ContainerAutohealCardProps) {
  const { t } = useTranslation('containers');
  // Read and write are gated separately, exactly as the backend gates them:
  // GET takes containers.manage OR autoheal.view, POST takes containers.manage
  // OR autoheal.manage. Firing the query without the read key would put a 403
  // on every container the user opens.
  const { allowed: canView, reason: viewReason } = usePermissionGate([PERM.CONTAINERS_MANAGE, PERM.AUTOHEAL_VIEW]);
  const { allowed: canManage, reason } = usePermissionGate([PERM.CONTAINERS_MANAGE, PERM.AUTOHEAL_MANAGE]);
  const { data, isLoading } = useAutohealPreference(containerId, canView);
  const setPreference = useSetAutohealPreference(containerId);

  return (
    <Box className="p-4">
      <Flex align="center" justify="between" gap={3}>
        <Flex align="center" gap={2}>
          <IconHeartbeat className="size-4 text-muted-foreground" aria-hidden="true" />
          <Subheading className="text-sm font-medium text-foreground">{t('autoheal.title')}</Subheading>
        </Flex>
        {canView && isLoading ? (
          <Skeleton className="h-5 w-10" />
        ) : (
          <Switch
            id={`autoheal-${containerId}`}
            checked={data?.enabled ?? false}
            onCheckedChange={(next) => setPreference.mutate(next)}
            disabled={!canView || !canManage || setPreference.isPending}
            aria-label={t('autoheal.enable')}
            title={canManage ? undefined : reason}
          />
        )}
      </Flex>

      <BodyText variant="muted" size="sm" className="mt-2">
        {t('autoheal.enableDesc')}
      </BodyText>

      {!canView && (
        <BodyText variant="muted" size="sm" className="mt-2">
          {viewReason}
        </BodyText>
      )}

      {/* The switch is the only control here, and a disabled Switch has
          nowhere to hang a tooltip the way <Button permission> does, so the
          missing permission is named in the body instead of left to guesswork. */}
      {!canManage && (
        <BodyText variant="muted" size="sm" className="mt-2">
          {reason}
        </BodyText>
      )}

      {canView && !isLoading && data && (
        <Flex align="center" gap={2} wrap className="mt-3">
          <Badge variant="secondary">{t('autoheal.restartCount', { count: data.restartCount })}</Badge>
          {data.lastHealAt && (
            <Badge variant="secondary">
              {t('autoheal.lastHeal')}: {formatDate(data.lastHealAt)}
            </Badge>
          )}
          {/* The engine skips a container it has never seen healthy, so the
              switch can read as broken without this. */}
          <Badge variant={data.wasEverHealthy ? 'secondary' : 'warning'}>
            {data.wasEverHealthy ? t('autoheal.wasHealthy') : t('autoheal.neverHealthy')}
          </Badge>
        </Flex>
      )}

      <BodyText variant="muted" size="sm" className="mt-3">
        {t('autoheal.footnote')}
      </BodyText>
    </Box>
  );
}
