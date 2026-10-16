// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowDown, IconArrowUp, IconCrown, IconTrash, IconWaterpolo } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { IconServer } from '@tabler/icons-react';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { StatusBadge } from '../components/StatusBadge';
import { useSetNodeAvailability, useSetNodeRole } from '../hooks/useSwarmNodes';
import { nodeAvailabilityTone, nodeStateTone, shortId } from '../format';
import type { SwarmNode } from '../types';

type Props = {
  node: SwarmNode;
  onEditLabels: () => void;
  onRemove: () => void;
};

export function NodeDetailHeader({ node, onEditLabels, onRemove }: Props) {
  const { t } = useTranslation('swarm');
  const setAvailability = useSetNodeAvailability();
  const setRole = useSetNodeRole();
  const draining = node.availability === 'drain';

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconServer className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            {node.isLeader && <IconCrown className="size-5 text-warning" aria-hidden />}
            <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
              {node.hostname || node.id}
            </h1>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <Badge variant={node.role === 'manager' ? 'default' : 'secondary'}>{t(`nodeRole.${node.role}`)}</Badge>
            <StatusBadge tone={nodeStateTone(node.state)}>{t(`nodeState.${node.state}`)}</StatusBadge>
            <StatusBadge tone={nodeAvailabilityTone(node.availability)}>
              {t(`nodeAvailability.${node.availability}`)}
            </StatusBadge>
            {node.self && <Badge variant="outline">{t('nodes.thisHost')}</Badge>}
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {node.addr || shortId(node.id)}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            permission={PERM.SWARM_NODES_MANAGE}
            onClick={() =>
              setAvailability.mutate({
                id: node.id,
                availability: draining ? 'active' : 'drain',
              })
            }
          >
            <IconWaterpolo className="size-4" />
            {draining ? t('actions.activate') : t('actions.drain')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            permission={PERM.SWARM_NODES_MANAGE}
            onClick={() =>
              setRole.mutate({
                id: node.id,
                role: node.role === 'manager' ? 'worker' : 'manager',
              })
            }
          >
            {node.role === 'manager' ? <IconArrowDown className="size-4" /> : <IconArrowUp className="size-4" />}
            {node.role === 'manager' ? t('actions.demote') : t('actions.promote')}
          </Button>
          <Button variant="outline" size="sm" permission={PERM.SWARM_NODES_MANAGE} onClick={() => onEditLabels()}>
            {t('actions.editLabels')}
          </Button>
          <Button variant="destructive" size="sm" permission={PERM.SWARM_NODES_DELETE} onClick={() => onRemove()}>
            <IconTrash className="size-4" />
            {t('actions.remove')}
          </Button>
        </div>
      </PageHeadActions>
    </>
  );
}
