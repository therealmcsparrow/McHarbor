// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLock, IconLockOpen } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { truncateId } from '@resources/utils/format';
import { containerStatePresentation } from '@resources/utils/container-state';
import type { ContainerInspect } from '@core/types/docker';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { ContainerIcon } from '@resources/components/ContainerIcon';

interface ContainerDetailIdentityProps {
  container: ContainerInspect;
  containerName: string;
  stackName: string | null;
  protectedContainer: boolean;
  editUnlocked: boolean;
}

export function ContainerDetailIdentity({
  container,
  containerName,
  stackName: resolvedStackName,
  protectedContainer,
  editUnlocked,
}: ContainerDetailIdentityProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const state = container.State?.Status ?? 'unknown';
  const statePresentation = containerStatePresentation(state, container.State?.Health?.Status);
  return (
    <PageHeadIdentity>
      {/* The image's own mark, not two letters of the container name.
          A homelab is a wall of familiar logos — postgres, redis, nginx —
          and the eye lands on the logo long before it parses "po".
          ContainerIcon falls back to a Tabler glyph, then to IconBox, so
          the tile is never empty. */}
      <span
        aria-hidden="true"
        className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
      >
        <ContainerIcon image={container.Config?.Image ?? container.Image ?? ''} className="size-6 text-primary" />
      </span>
      <div className="min-w-0">
        <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
          {containerName}
        </h1>
        <div className="mt-1 flex flex-wrap items-center gap-1.5">
          <StatusIndicator tone={statePresentation.tone} label={state} size="sm" pulse={state === 'running'} />
          {protectedContainer && (
            <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
              {editUnlocked ? <IconLockOpen className="size-3" /> : <IconLock className="size-3" />}
              {editUnlocked ? t('actions.editingUnlocked') : t('actions.locked', { ns: 'common' })}
            </Badge>
          )}
          {resolvedStackName ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-auto bg-primary/10 px-1.5 py-0 text-[10px] text-primary hover:bg-primary/20 focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => navigate(`/stacks/${resolvedStackName}`)}
            >
              {resolvedStackName}
            </Button>
          ) : (
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
              {t('badges.standalone')}
            </Badge>
          )}
          {/* Image and id as mono chips, on the same line as the state.
            These are identifiers, not prose — the mono face and the
            chip make them scannable and copy-able at a glance. */}
          {container.Config?.Image && (
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {container.Config.Image}
            </span>
          )}
          <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
            {truncateId(container.Id)}
          </span>
        </div>
      </div>
    </PageHeadIdentity>
  );
}
