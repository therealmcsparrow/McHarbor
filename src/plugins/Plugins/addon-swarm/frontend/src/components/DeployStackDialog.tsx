// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
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
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import { Spinner } from '@resources/components/ui/Spinner';
import { statusSoft } from '@resources/theme/status-palette';
import { SwarmDeployRejectedError, useDeploySwarmStack } from '../hooks/useSwarmStacks';
import type { SwarmStackServiceError } from '../types';

const NAME_PLACEHOLDER = 'my-stack';
const COMPOSE_PLACEHOLDER = 'version: "3.8"\nservices:\n  web:\n    image: nginx:alpine';

/** Rejection reasons come from the backend's per-service validation, so an unknown one shows verbatim. */
function reasonLabel(t: (key: string, options?: Record<string, unknown>) => string, reason: string): string {
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: reasons are emitted by the stack validator and grow with it
  return t(`dialogs.deployStack.reasons.${reason}`, { defaultValue: reason });
}

type DeployStackDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-fill for a redeploy of an existing stack. */
  initialName?: string;
};

export function DeployStackDialog({ open: isOpen, onOpenChange, initialName }: DeployStackDialogProps) {
  const { t } = useTranslation('swarm');
  const deploy = useDeploySwarmStack();
  const [itemName, setName] = useState(initialName ?? '');
  const [compose, setCompose] = useState('');
  const [prune, setPrune] = useState(false);
  const [rejected, setRejected] = useState<SwarmStackServiceError[]>([]);

  const canSubmit = itemName.trim() !== '' && compose.trim() !== '';

  const closeDialog = (next: boolean) => {
    if (!next) {
      setRejected([]);
    }
    onOpenChange(next);
  };

  const submit = () => {
    setRejected([]);
    deploy.mutate(
      { name: itemName.trim(), compose, prune },
      {
        onSuccess: () => {
          setCompose('');
          closeDialog(false);
        },
        onError: (error) => {
          // A rejection is the operator's file being wrong, and the server
          // said exactly which keys. Keeping the dialog open with the list
          // attached is the difference between a fixable error and a toast.
          if (error instanceof SwarmDeployRejectedError) setRejected(error.rejected);
        },
      },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={closeDialog}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('dialogs.deployStack.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.deployStack.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <label className="block space-y-1.5">
            <span className="text-sm font-medium text-foreground">{t('fields.stackName')}</span>
            <Input value={itemName} onChange={(e) => setName(e.target.value)} placeholder={NAME_PLACEHOLDER} />
          </label>

          <label className="block space-y-1.5">
            <span className="text-sm font-medium text-foreground">{t('fields.compose')}</span>
            <Textarea
              value={compose}
              onChange={(e) => setCompose(e.target.value)}
              rows={16}
              className="font-mono text-xs"
              placeholder={COMPOSE_PLACEHOLDER}
            />
          </label>

          <label htmlFor="swarm-deploy-prune" className="flex items-start gap-3">
            <Switch id="swarm-deploy-prune" checked={prune} onCheckedChange={setPrune} />
            <span className="space-y-0.5">
              <span className="block text-sm text-foreground">{t('fields.prune')}</span>
              <span className="block text-xs text-muted-foreground">{t('fields.pruneHint')}</span>
            </span>
          </label>

          {rejected.length > 0 && (
            <div className={`space-y-2 rounded-lg p-4 ${statusSoft('danger')}`}>
              <p className="flex items-center gap-2 text-sm font-medium">
                <IconAlertTriangle className="size-4" />
                {t('dialogs.deployStack.rejectedTitle')}
              </p>
              <ul className="space-y-1 text-sm">
                {rejected.map((item) => (
                  <li key={`${item.service}:${item.reason}`}>
                    <span className="font-medium">{item.service}</span>
                    <span className="opacity-90">
                      {' — '}
                      {reasonLabel(t, item.reason)}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => closeDialog(false)}>
            {t('actions.cancel')}
          </Button>
          <Button permission={PERM.SWARM_STACKS_MANAGE} disabled={!canSubmit || deploy.isPending} onClick={submit}>
            {deploy.isPending && <Spinner size="sm" />}
            {t('actions.deploy')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
