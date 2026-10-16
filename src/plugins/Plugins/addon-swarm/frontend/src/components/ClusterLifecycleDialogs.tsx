// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
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
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { useSwarmInit, useSwarmJoin } from '../hooks/useSwarmCluster';
import { Field } from './ClusterField';
import type { DialogProps } from './ClusterField';

const JOIN_TOKEN_PLACEHOLDER = 'SWMTKN-1-...';

/** Initialise a new single-node Swarm on this environment. */
export function InitClusterDialog({ open: isOpen, onOpenChange }: DialogProps) {
  const { t } = useTranslation('swarm');
  const init = useSwarmInit();
  const [advertiseAddr, setAdvertiseAddr] = useState('');
  const [listenAddr, setListenAddr] = useState('');
  const [autoLock, setAutoLock] = useState(false);

  const submit = () => {
    init.mutate(
      {
        advertiseAddr: advertiseAddr.trim() || undefined,
        listenAddr: listenAddr.trim() || undefined,
        autoLockManagers: autoLock,
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialogs.init.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.init.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <Field
            label={t('fields.advertiseAddr')}
            hint={t('fields.advertiseAddrHint')}
            value={advertiseAddr}
            onChange={setAdvertiseAddr}
            placeholder="192.168.1.10"
          />
          <Field
            label={t('fields.listenAddr')}
            hint={t('fields.listenAddrHint')}
            value={listenAddr}
            onChange={setListenAddr}
            placeholder="0.0.0.0:2377"
          />
          <label htmlFor="swarm-init-autolock" className="flex items-start gap-3">
            <Switch id="swarm-init-autolock" checked={autoLock} onCheckedChange={setAutoLock} />
            <span className="space-y-0.5">
              <span className="block text-sm text-foreground">{t('fields.autoLock')}</span>
              <span className="block text-xs text-muted-foreground">{t('fields.autoLockHint')}</span>
            </span>
          </label>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button permission={PERM.SWARM_MANAGE} onClick={submit} disabled={init.isPending}>
            {init.isPending && <Spinner size="sm" />}
            {t('actions.initCluster')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Join this environment to an existing Swarm. */
export function JoinClusterDialog({ open: isOpen, onOpenChange }: DialogProps) {
  const { t } = useTranslation('swarm');
  const join = useSwarmJoin();
  const [remoteAddr, setRemoteAddr] = useState('');
  const [joinToken, setJoinToken] = useState('');
  const [advertiseAddr, setAdvertiseAddr] = useState('');

  const canSubmit = remoteAddr.trim() !== '' && joinToken.trim() !== '';

  const submit = () => {
    join.mutate(
      {
        remoteAddrs: remoteAddr
          .split(',')
          .map((a) => a.trim())
          .filter(Boolean),
        joinToken: joinToken.trim(),
        advertiseAddr: advertiseAddr.trim() || undefined,
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialogs.join.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.join.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <Field
            label={t('fields.remoteAddrs')}
            hint={t('fields.remoteAddrsHint')}
            value={remoteAddr}
            onChange={setRemoteAddr}
            placeholder="192.168.1.10:2377"
          />
          <Field
            label={t('fields.joinToken')}
            hint={t('fields.joinTokenHint')}
            value={joinToken}
            onChange={setJoinToken}
            placeholder={JOIN_TOKEN_PLACEHOLDER}
          />
          <Field
            label={t('fields.advertiseAddr')}
            hint={t('fields.advertiseAddrHint')}
            value={advertiseAddr}
            onChange={setAdvertiseAddr}
            placeholder="192.168.1.11"
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button permission={PERM.SWARM_MANAGE} onClick={submit} disabled={!canSubmit || join.isPending}>
            {join.isPending && <Spinner size="sm" />}
            {t('actions.joinCluster')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
