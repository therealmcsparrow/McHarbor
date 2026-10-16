// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCopy } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  useSwarmJoinTokens,
  useSwarmRotateJoinToken,
  useSwarmRotateUnlockKey,
  useSwarmUnlockKey,
} from '../hooks/useSwarmCluster';
import type { DialogProps } from './ClusterField';

/**
 * Show the join tokens.
 *
 * The query is enabled only while this dialog is open: possession of the
 * manager token is control of the cluster, and the read is audited.
 */
export function JoinTokensDialog({ open: isOpen, onOpenChange }: DialogProps) {
  const { t } = useTranslation('swarm');
  const tokens = useSwarmJoinTokens(isOpen);
  const rotate = useSwarmRotateJoinToken();

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialogs.tokens.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.tokens.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {tokens.isLoading && <Spinner size="sm" />}
          {tokens.data && (
            <>
              <SecretRow
                label={t('fields.workerToken')}
                value={tokens.data.worker}
                onRotate={() => rotate.mutate('worker')}
                rotateLabel={t('actions.rotate')}
                rotating={rotate.isPending}
                copyLabel={t('actions.copy')}
              />
              <SecretRow
                label={t('fields.managerToken')}
                value={tokens.data.manager}
                onRotate={() => rotate.mutate('manager')}
                rotateLabel={t('actions.rotate')}
                rotating={rotate.isPending}
                copyLabel={t('actions.copy')}
              />
            </>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Show and rotate the autolock key. */
export function UnlockKeyDialog({ open: isOpen, onOpenChange }: DialogProps) {
  const { t } = useTranslation('swarm');
  const key = useSwarmUnlockKey(isOpen);
  const rotate = useSwarmRotateUnlockKey();

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialogs.unlockKey.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.unlockKey.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {key.isLoading && <Spinner size="sm" />}
          {key.data && (
            <SecretRow
              label={t('fields.unlockKey')}
              value={key.data.key}
              onRotate={() => rotate.mutate()}
              rotateLabel={t('actions.rotate')}
              rotating={rotate.isPending}
              copyLabel={t('actions.copy')}
            />
          )}
          {key.data && key.data.key === '' && (
            <p className="text-sm text-muted-foreground">{t('dialogs.unlockKey.notLocked')}</p>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// --- shared bits -----------------------------------------------------------

/**
 * A credential and the two things anyone ever does with one.
 *
 * The value is shown in full rather than masked: this dialog is already behind
 * `swarm.manage` and an audited read, and a masked token you cannot read is a
 * token you cannot use for the one job you opened the dialog to do.
 */
function SecretRow({
  label,
  value,
  onRotate,
  rotateLabel,
  rotating,
  copyLabel,
}: {
  label: string;
  value: string;
  onRotate: () => void;
  rotateLabel: string;
  rotating: boolean;
  copyLabel: string;
}) {
  return (
    <div className="space-y-1.5">
      <span className="text-sm font-medium text-foreground">{label}</span>
      <div className="flex items-start gap-2">
        <code className="min-w-0 flex-1 rounded-lg bg-muted px-3 py-2 font-mono text-xs break-all text-foreground">
          {value}
        </code>
        <Button
          variant="ghost"
          size="icon"
          aria-label={copyLabel}
          onClick={() => void navigator.clipboard?.writeText(value)}
        >
          <IconCopy className="size-4" />
        </Button>
        <Button variant="outline" size="sm" permission={PERM.SWARM_MANAGE} onClick={onRotate} disabled={rotating}>
          {rotateLabel}
        </Button>
      </div>
    </div>
  );
}
