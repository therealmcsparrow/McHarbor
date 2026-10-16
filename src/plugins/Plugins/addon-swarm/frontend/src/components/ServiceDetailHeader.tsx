// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowBackUp, IconRefresh, IconResize, IconTrash } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { IconStack2 } from '@tabler/icons-react';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusBadge } from '../components/StatusBadge';
import { useRestartSwarmService, useRollbackSwarmService } from '../hooks/useSwarmServices';
import { replicaText, serviceTone, updateStateTone } from '../format';
import type { SwarmServiceDetail } from '../types';

type Props = {
  service: SwarmServiceDetail;
  onScale: () => void;
  onRemove: () => void;
};

export function ServiceDetailHeader({ service, onScale, onRemove }: Props) {
  const { t } = useTranslation('swarm');
  const restart = useRestartSwarmService();
  const rollback = useRollbackSwarmService();

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconStack2 className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {service.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            {service.stackName && <Badge variant="outline">{service.stackName}</Badge>}
            <Badge variant="secondary">{t(`serviceMode.${service.mode}`)}</Badge>
            <StatusBadge tone={serviceTone(service)}>{replicaText(service)}</StatusBadge>
            {service.updateState && (
              <StatusBadge tone={updateStateTone(service.updateState)}>
                {t(`updateState.${service.updateState}`)}
              </StatusBadge>
            )}
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {service.image.split('@')[0]}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            permission={PERM.SWARM_SERVICES_SCALE}
            disabled={!service.mode.startsWith('replicated')}
            onClick={() => onScale()}
          >
            <IconResize className="size-4" />
            {t('actions.scale')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            permission={PERM.SWARM_SERVICES_MANAGE}
            disabled={restart.isPending}
            onClick={() => restart.mutate(service.id)}
          >
            {restart.isPending ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
            {t('actions.restart')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            permission={PERM.SWARM_SERVICES_MANAGE}
            // A service with only one spec has nothing to go back to, and
            // the backend answers 409 for it — the disabled state says so
            // before the click rather than after.
            disabled={!service.hasPreviousSpec || rollback.isPending}
            onClick={() => rollback.mutate(service.id)}
          >
            <IconArrowBackUp className="size-4" />
            {t('actions.rollback')}
          </Button>
          <Button variant="destructive" size="sm" permission={PERM.SWARM_SERVICES_DELETE} onClick={() => onRemove()}>
            <IconTrash className="size-4" />
            {t('actions.remove')}
          </Button>
        </div>
      </PageHeadActions>
    </>
  );
}
