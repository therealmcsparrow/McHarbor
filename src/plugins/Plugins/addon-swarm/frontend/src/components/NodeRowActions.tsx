// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconArrowDown,
  IconArrowUp,
  IconPlayerPause,
  IconPlayerPlay,
  IconTrash,
  IconWaterpolo,
} from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useSetNodeAvailability, useSetNodeRole } from '../hooks/useSwarmNodes';
import type { SwarmNode } from '../types';

type Props = {
  node: SwarmNode;
  setAvailability: ReturnType<typeof useSetNodeAvailability>;
  setRole: ReturnType<typeof useSetNodeRole>;
  onRemove: (node: SwarmNode) => void;
};

export function NodeRowActions({ node, setAvailability, setRole, onRemove }: Props) {
  const { t } = useTranslation('swarm');
  const draining = node.availability === 'drain';

  return (
    <div className="flex items-center justify-end gap-1">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={draining ? t('actions.activate') : t('actions.drain')}
        permission={PERM.SWARM_NODES_MANAGE}
        onClick={(e) => {
          e.stopPropagation();
          setAvailability.mutate({
            id: node.id,
            availability: draining ? 'active' : 'drain',
          });
        }}
      >
        {draining ? <IconPlayerPlay className="size-4" /> : <IconWaterpolo className="size-4" />}
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={node.availability === 'pause' ? t('actions.activate') : t('actions.pause')}
        permission={PERM.SWARM_NODES_MANAGE}
        onClick={(e) => {
          e.stopPropagation();
          setAvailability.mutate({
            id: node.id,
            availability: node.availability === 'pause' ? 'active' : 'pause',
          });
        }}
      >
        <IconPlayerPause className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={node.role === 'manager' ? t('actions.demote') : t('actions.promote')}
        permission={PERM.SWARM_NODES_MANAGE}
        onClick={(e) => {
          e.stopPropagation();
          setRole.mutate({
            id: node.id,
            role: node.role === 'manager' ? 'worker' : 'manager',
          });
        }}
      >
        {node.role === 'manager' ? <IconArrowDown className="size-4" /> : <IconArrowUp className="size-4" />}
      </Button>
      {/* SwarmKit has no path for a node to remove itself; leaving
                  the cluster is that operation. Saying so on the control the
                  operator is reaching for beats a 409 after the click. */}
      {node.self ? (
        <Tooltip>
          <TooltipTrigger asChild>
            {/* A disabled button emits no pointer events, so the
                        trigger has to be the span wrapping it. */}
            <span className="inline-flex">
              <Button variant="ghost" size="icon-sm" aria-label={t('actions.remove')} disabled>
                <IconTrash className="size-4" />
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{t('nodes.cannotRemoveSelf')}</TooltipContent>
        </Tooltip>
      ) : (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.remove')}
          permission={PERM.SWARM_NODES_DELETE}
          onClick={(e) => {
            e.stopPropagation();
            onRemove(node);
          }}
        >
          <IconTrash className="size-4" />
        </Button>
      )}
    </div>
  );
}
