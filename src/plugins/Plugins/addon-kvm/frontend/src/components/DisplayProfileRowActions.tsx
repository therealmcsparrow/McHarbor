// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy, IconPencil, IconTrash } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { useDeleteDisplayProfile, useDuplicateDisplayProfile } from '../hooks/useDisplayProfiles';
import type { DisplayProfile } from '../types-display';
import { profileName } from './displayProfileLabels';

interface ActionProps {
  label: string;
  readOnlyReason?: string;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}

/**
 * A built-in's edit and delete stay visible but disabled, with the reason as
 * the tooltip; a missing vm.manage is named by the Button's own gate.
 */
function Action({ label, readOnlyReason, disabled = false, onClick, children }: ActionProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            permission={VM_PERM.MANAGE}
            disabled={disabled || Boolean(readOnlyReason)}
            onClick={onClick}
          >
            {children}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{readOnlyReason ?? label}</TooltipContent>
    </Tooltip>
  );
}

export function DisplayProfileRowActions({
  profile,
  onEdit,
}: {
  profile: DisplayProfile;
  onEdit: (profile: DisplayProfile) => void;
}) {
  const { t } = useTranslation('vm');
  const duplicate = useDuplicateDisplayProfile();
  const remove = useDeleteDisplayProfile();
  const [confirming, setConfirming] = useState(false);
  // isPending reaches the button a render late; a second click in that window
  // must not send a second delete.
  const sending = useRef(false);
  const readOnly = profile.builtin ? t('displayProfiles.builtinReadOnly') : undefined;
  const inUse = profile.vmCount > 0;

  const confirmDelete = () => {
    if (sending.current) return;
    sending.current = true;
    remove.mutate(
      { id: profile.id, reassign: inUse },
      {
        onSuccess: () => setConfirming(false),
        onSettled: () => {
          sending.current = false;
        },
      },
    );
  };

  return (
    <div className="flex items-center justify-end gap-1">
      <Action label={t('displayProfiles.actions.edit')} readOnlyReason={readOnly} onClick={() => onEdit(profile)}>
        <IconPencil className="h-4 w-4" aria-hidden="true" />
      </Action>
      <Action
        label={t('displayProfiles.actions.duplicate')}
        disabled={duplicate.isPending}
        onClick={() => duplicate.mutate(profile.id)}
      >
        <IconCopy className="h-4 w-4" aria-hidden="true" />
      </Action>
      <Action
        label={t('displayProfiles.actions.delete')}
        readOnlyReason={readOnly}
        disabled={remove.isPending}
        onClick={() => {
          remove.reset();
          setConfirming(true);
        }}
      >
        <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
      </Action>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(isOpen) => !remove.isPending && setConfirming(isOpen)}
        title={t('displayProfiles.delete.title', { name: profileName(t, profile) })}
        description={
          inUse ? t('displayProfiles.delete.bodyInUse', { count: profile.vmCount }) : t('displayProfiles.delete.body')
        }
        confirmLabel={inUse ? t('displayProfiles.delete.reassignConfirm') : t('displayProfiles.delete.confirm')}
        permission={VM_PERM.MANAGE}
        loading={remove.isPending}
        onConfirm={confirmDelete}
      >
        {remove.isError && <Alert variant="destructive" description={remove.error.message} />}
      </ConfirmDialog>
    </div>
  );
}
