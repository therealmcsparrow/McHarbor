// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconArrowLeft,
  IconCheck,
  IconLoader2,
  IconLock,
  IconLockOpen,
  IconSettings,
  IconShield,
  IconX,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PERM } from '@core/auth/permissionKeys';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { ContainerIcon } from '@resources/components/ContainerIcon';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { containerStatePresentation } from '@resources/utils/container-state';
import { truncateId } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';

type ContainerEditHeaderProps = {
  container: ContainerInspect;
  isProtected: boolean;
  canUnlock: boolean;
  editLocked: boolean;
  editUnlocked: boolean;
  onBack: () => void;
  onToggleLock: () => void;
  /** Present while the form is editable; the page owns the edit lifecycle. */
  editing: boolean;
  dirty: boolean;
  saving: boolean;
  onSave: () => void;
  onCancel: () => void;
};

export function ContainerEditHeader({
  container,
  isProtected,
  canUnlock,
  editLocked,
  editUnlocked,
  onBack,
  onToggleLock,
  editing,
  dirty,
  saving,
  onSave,
  onCancel,
}: ContainerEditHeaderProps) {
  const { t } = useTranslation('containers');
  const containerName = (container.Name ?? '').replace(/^\//, '');
  const state = container.State?.Status ?? 'unknown';
  const presentation = containerStatePresentation(state, container.State?.Health?.Status);

  return (
    <>
      <PageHeadIdentity>
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
            <Badge variant="default" className="gap-1 px-1.5 py-0 text-[10px]">
              <IconSettings className="size-3" />
              {t('configEdit.badge')}
            </Badge>
            <StatusIndicator tone={presentation.tone} label={state} size="sm" pulse={state === 'running'} />
            {isProtected && (
              <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                {editLocked ? <IconLock className="size-3" /> : <IconLockOpen className="size-3" />}
                {editLocked ? t('configEdit.protected.locked') : t('configEdit.protected.unlocked')}
              </Badge>
            )}
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

      <PageHeadActions>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          {canUnlock && (
            <HeaderActionButton
              tooltip={editUnlocked ? t('configEdit.protected.lock') : t('configEdit.protected.unlock')}
              onClick={onToggleLock}
              variant={editUnlocked ? 'secondary' : 'outline'}
              icon={
                editUnlocked ? <IconLockOpen className="size-3.5 text-success" /> : <IconShield className="size-3.5" />
              }
            />
          )}
          {editing ? (
            <>
              <HeaderActionButton
                tooltip={t('edit.saveChanges')}
                permission={PERM.CONTAINERS_MANAGE}
                disabled={!dirty || saving}
                onClick={onSave}
                icon={saving ? <IconLoader2 className="size-3.5 animate-spin" /> : <IconCheck className="size-3.5" />}
                className="text-success hover:border-success/30 hover:bg-success/10"
              />
              <HeaderActionButton
                tooltip={t('edit.cancelChanges')}
                disabled={saving}
                onClick={onCancel}
                icon={<IconX className="size-3.5" />}
                className="text-muted-foreground hover:border-border hover:bg-muted/50"
              />
            </>
          ) : (
            <HeaderActionButton
              tooltip={t('configEdit.backToDetails')}
              onClick={onBack}
              icon={<IconArrowLeft className="size-3.5" />}
            />
          )}
        </div>
      </PageHeadActions>
    </>
  );
}
