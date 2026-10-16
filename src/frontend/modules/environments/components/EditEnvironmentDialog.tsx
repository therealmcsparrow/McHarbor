// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import { DockerConnectionFields } from './DockerConnectionFields';
import { useUpdateEnvironment } from '../hooks/useEnvironmentActions';
import type { EnvironmentInfo } from '../hooks/useEnvironments';

interface EditEnvironmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  env: EnvironmentInfo;
}

export function EditEnvironmentDialog({ open: isOpen, onOpenChange, env }: EditEnvironmentDialogProps) {
  const { t } = useTranslation('environments');
  const updateEnv = useUpdateEnvironment();

  const [envName, setEnvName] = useState(env.name);
  const [connType, setConnType] = useState(env.connectionType);
  // docker socket: default value of the field the operator types into, used only when the server sent none.
  const [socketPath, setSocketPath] = useState(env.socketPath ?? '/var/run/docker.sock');
  const [host, setHost] = useState(env.host ?? '');
  const [port, setPort] = useState(String(env.port ?? 2375));
  const [isDefault, setIsDefault] = useState(env.isDefault);
  const [isActive, setIsActive] = useState(env.isActive);

  const [syncedOpen, setSyncedOpen] = useState(isOpen);
  const [syncedEnv, setSyncedEnv] = useState(env);
  if (isOpen !== syncedOpen || env !== syncedEnv) {
    setSyncedOpen(isOpen);
    setSyncedEnv(env);
    if (isOpen) {
      setEnvName(env.name);
      setConnType(env.connectionType);
      setSocketPath(env.socketPath ?? '/var/run/docker.sock');
      setHost(env.host ?? '');
      setPort(String(env.port ?? 2375));
      setIsDefault(env.isDefault);
      setIsActive(env.isActive);
    }
  }

  const trimmedName = envName.trim();
  const canSave = trimmedName.length > 0 && !updateEnv.isPending;

  function handleSave() {
    if (!canSave) return;
    const data: Record<string, unknown> = {
      name: trimmedName,
      connectionType: connType,
      isDefault,
      isActive,
    };
    if (connType === 'socket') {
      data.socketPath = socketPath.trim();
    }
    if (connType === 'tcp' || connType === 'tls') {
      data.host = host.trim();
      data.port = Number(port) || 0;
    }
    updateEnv.mutate({ id: env.id, data }, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('edit.title')}</DialogTitle>
          <DialogDescription>{t('edit.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="edit-env-name">{t('create.name')}</Label>
            <Input
              id="edit-env-name"
              value={envName}
              onChange={(e) => setEnvName(e.target.value)}
              placeholder={t('create.namePlaceholderDocker')}
            />
          </div>

          <DockerConnectionFields
            connType={connType}
            onConnTypeChange={setConnType}
            socketPath={socketPath}
            onSocketPathChange={setSocketPath}
            host={host}
            onHostChange={setHost}
            port={port}
            onPortChange={setPort}
          />

          <div className="flex items-center justify-between border-t border-border pt-4">
            <div className="space-y-0.5">
              <Label htmlFor="edit-env-default">{t('edit.setDefault')}</Label>
              <p className="text-xs text-muted-foreground">{t('edit.setDefaultHint')}</p>
            </div>
            <Switch id="edit-env-default" checked={isDefault} onCheckedChange={setIsDefault} />
          </div>

          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="edit-env-active">{t('edit.active')}</Label>
              <p className="text-xs text-muted-foreground">{t('edit.activeHint')}</p>
            </div>
            <Switch id="edit-env-active" checked={isActive} onCheckedChange={setIsActive} />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.ENVIRONMENTS_MANAGE} onClick={handleSave} disabled={!canSave}>
            {updateEnv.isPending ? t('actions.saving', { ns: 'common' }) : t('actions.save', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
