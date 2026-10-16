// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

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
import type { InstallTokenResponse } from '../hooks/useEnvironmentActions';
import { useCreateEnvironmentFields } from '../hooks/useCreateEnvironmentFields';
import { useCreateEnvironmentSubmit } from '../hooks/useCreateEnvironmentSubmit';
import { DockerConnectionFields } from './DockerConnectionFields';
import { CreateEnvironmentSshSection } from './CreateEnvironmentSshSection';
import { CreateEnvironmentAgentImport } from './CreateEnvironmentAgentImport';
import { PERM } from '@core/auth/permissionKeys';

interface CreateEnvironmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAgentToken: (token: string) => void;
  onInstallScript?: (data: InstallTokenResponse) => void;
}

export function CreateEnvironmentDialog({
  open: isOpen,
  onOpenChange,
  onAgentToken,
  onInstallScript,
}: CreateEnvironmentDialogProps) {
  const { t } = useTranslation('environments');
  const f = useCreateEnvironmentFields();
  const submit = useCreateEnvironmentSubmit(f, { onOpenChange, onAgentToken, onInstallScript });

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(next) => {
        if (!next) f.resetForm();
        onOpenChange(next);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('create.name')}</Label>
            <Input
              type="text"
              value={f.envName}
              onChange={(e) => f.setEnvName(e.target.value)}
              placeholder={t('create.namePlaceholderDocker')}
            />
          </div>

          <>
            <DockerConnectionFields
              connType={f.connType}
              onConnTypeChange={f.setConnType}
              socketPath={f.socketPath}
              onSocketPathChange={f.setSocketPath}
              host={f.host}
              onHostChange={f.setHost}
              port={f.port}
              onPortChange={f.setPort}
              deployMethod={f.deployMethod}
              onDeployMethodChange={f.setDeployMethod}
              sshHost={f.sshHost}
              onSSHHostChange={f.setSSHHost}
              sshPort={f.sshPort}
              onSSHPortChange={f.setSSHPort}
              sshUser={f.sshUser}
              onSSHUserChange={f.setSSHUser}
              hostKeyFingerprint={f.hostKeyFingerprint}
              onHostKeyFingerprintChange={f.setHostKeyFingerprint}
              sshAuthType={f.sshAuthType}
              onSSHAuthTypeChange={f.setSSHAuthType}
              sshKey={f.sshKey}
              onSSHKeyChange={f.setSSHKey}
              sshPassword={f.sshPassword}
              onSSHPasswordChange={f.setSSHPassword}
              sshDeployAs={f.sshDeployAs}
              onSSHDeployAsChange={f.setSSHDeployAs}
            />

            {f.deployMethod === 'ssh' && (
              <CreateEnvironmentSshSection
                f={f}
                probing={submit.probing}
                installing={submit.installing}
                onProbe={submit.handleProbePrereqs}
                onInstall={submit.handleInstallPrereqs}
              />
            )}

            {f.connType === 'agent' && <CreateEnvironmentAgentImport f={f} />}
          </>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              onOpenChange(false);
              f.resetForm();
            }}
          >
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.ENVIRONMENTS_MANAGE}
            onClick={submit.handleCreate}
            disabled={
              submit.isPending ||
              !f.envName.trim() ||
              (f.connType === 'agent' && f.deployMethod === 'ssh' && !f.isSSHDeployReady) ||
              (f.connType === 'agent' && f.deployMethod === 'ssh' && !f.prereqGate.ready) ||
              (f.connType === 'agent' && f.importAgentToken && f.existingAgentToken.trim() === '')
            }
          >
            {submit.isPending ? t('create.adding') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
