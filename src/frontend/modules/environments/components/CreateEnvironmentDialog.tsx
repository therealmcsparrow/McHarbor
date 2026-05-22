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
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { useCreateEnvironment, useDeployAgent, useCreateInstallToken } from '../hooks/useEnvironmentActions';
import type { CreateEnvironmentData, InstallTokenResponse } from '../hooks/useEnvironmentActions';
import { DockerConnectionFields } from './DockerConnectionFields';
import { KubernetesConnectionFields } from './KubernetesConnectionFields';

type DeployMethod = 'manual' | 'ssh' | 'script';

interface CreateEnvironmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAgentToken: (token: string) => void;
  onInstallScript?: (data: InstallTokenResponse) => void;
}

export function CreateEnvironmentDialog({ open, onOpenChange, onAgentToken, onInstallScript }: CreateEnvironmentDialogProps) {
  const { t } = useTranslation('environments');
  const { t: tc } = useTranslation('common');
  const createEnv = useCreateEnvironment();
  const deployAgent = useDeployAgent();
  const createInstallToken = useCreateInstallToken();

  const [orchestratorType, setOrchestratorType] = useState<'docker' | 'kubernetes'>('docker');
  const [envName, setEnvName] = useState('');
  const [connType, setConnType] = useState('socket');
  const [socketPath, setSocketPath] = useState('/var/run/docker.sock');
  const [host, setHost] = useState('');
  const [port, setPort] = useState('2375');
  const [kubeconfig, setKubeconfig] = useState('');
  const [k8sNamespace, setK8sNamespace] = useState('default');
  const [k8sServerUrl, setK8sServerUrl] = useState('');
  const [k8sBearerToken, setK8sBearerToken] = useState('');
  const [k8sConnMethod, setK8sConnMethod] = useState<'kubeconfig' | 'bearer'>('kubeconfig');

  // Agent deploy fields
  const [deployMethod, setDeployMethod] = useState<DeployMethod>('manual');
  const [sshHost, setSSHHost] = useState('');
  const [sshPort, setSSHPort] = useState('22');
  const [sshUser, setSSHUser] = useState('root');
  const [hostKeyFingerprint, setHostKeyFingerprint] = useState('');
  const [sshAuthType, setSSHAuthType] = useState<'key' | 'password'>('key');
  const [sshKey, setSSHKey] = useState('');
  const [sshPassword, setSSHPassword] = useState('');
  const [sshDeployAs, setSSHDeployAs] = useState<'docker' | 'binary'>('docker');

  const resetForm = () => {
    setEnvName('');
    setOrchestratorType('docker');
    setConnType('socket');
    setSocketPath('/var/run/docker.sock');
    setHost('');
    setPort('2375');
    setKubeconfig('');
    setK8sNamespace('default');
    setK8sServerUrl('');
    setK8sBearerToken('');
    setK8sConnMethod('kubeconfig');
    setDeployMethod('manual');
    setSSHHost('');
    setSSHPort('22');
    setSSHUser('root');
    setHostKeyFingerprint('');
    setSSHAuthType('key');
    setSSHKey('');
    setSSHPassword('');
    setSSHDeployAs('docker');
  };

  const isSSHDeployReady =
    sshHost.trim() !== '' &&
    sshUser.trim() !== '' &&
    hostKeyFingerprint.trim() !== '' &&
    ((sshAuthType === 'key' && sshKey.trim() !== '') || (sshAuthType === 'password' && sshPassword !== ''));

  const handleCreate = () => {
    if (!envName.trim()) return;

    if (orchestratorType === 'kubernetes') {
      const data: CreateEnvironmentData = {
        name: envName.trim(),
        orchestratorType: 'kubernetes',
        k8sNamespace: k8sNamespace || 'default',
      };
      if (k8sConnMethod === 'kubeconfig') {
        data.kubeconfig = kubeconfig;
      } else {
        data.k8sServerUrl = k8sServerUrl;
        data.k8sBearerToken = k8sBearerToken;
      }
      createEnv.mutate(data, {
        onSuccess: () => { onOpenChange(false); resetForm(); },
      });
    } else {
      const data: CreateEnvironmentData = {
        name: envName.trim(),
        orchestratorType: 'docker',
        connectionType: connType,
      };
      if (connType === 'socket') {
        data.socketPath = socketPath;
      } else if (connType !== 'agent') {
        data.host = host;
        data.port = parseInt(port) || 2375;
      }
      createEnv.mutate(data, {
        onSuccess: (resp) => {
          onOpenChange(false);
          resetForm();
          if (resp && typeof resp === 'object' && 'agentToken' in resp && resp.agentToken) {
            const agentToken = resp.agentToken as string;
            const envId = (resp as { environment?: { id: string } }).environment?.id;

            if (deployMethod === 'ssh' && envId) {
              // Auto-deploy via SSH after environment creation
              deployAgent.mutate({
                envId,
                data: {
                  sshHost,
                  sshPort: parseInt(sshPort) || 22,
                  sshUser,
                  hostKeyFingerprint,
                  sshAuthType,
                  sshKey: sshAuthType === 'key' ? sshKey : undefined,
                  sshPassword: sshAuthType === 'password' ? sshPassword : undefined,
                  method: sshDeployAs,
                },
              });
              // Still show the token dialog as backup
              onAgentToken(agentToken);
            } else if (deployMethod === 'script' && envId) {
              // Generate install script token
              createInstallToken.mutate(envId, {
                onSuccess: (tokenResp) => {
                  onInstallScript?.(tokenResp);
                },
              });
              // Also show the agent token
              onAgentToken(agentToken);
            } else {
              // Manual — just show the token
              onAgentToken(agentToken);
            }
          }
        },
      });
    }
  };

  const isPending = createEnv.isPending || deployAgent.isPending;

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) resetForm(); onOpenChange(isOpen); }}>
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
              value={envName}
              onChange={(e) => setEnvName(e.target.value)}
              placeholder={orchestratorType === 'kubernetes' ? t('create.namePlaceholderK8s') : t('create.namePlaceholderDocker')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('create.platformLabel')}</Label>
            <Select
              value={orchestratorType}
              onChange={(v) => setOrchestratorType(v as 'docker' | 'kubernetes')}
              options={[
                { value: 'docker', label: t('platform.docker') },
                { value: 'kubernetes', label: t('platform.kubernetes') },
              ]}
            />
          </div>

          {orchestratorType === 'docker' ? (
            <DockerConnectionFields
              connType={connType}
              onConnTypeChange={setConnType}
              socketPath={socketPath}
              onSocketPathChange={setSocketPath}
              host={host}
              onHostChange={setHost}
              port={port}
              onPortChange={setPort}
              deployMethod={deployMethod}
              onDeployMethodChange={setDeployMethod}
              sshHost={sshHost}
              onSSHHostChange={setSSHHost}
              sshPort={sshPort}
              onSSHPortChange={setSSHPort}
              sshUser={sshUser}
              onSSHUserChange={setSSHUser}
              hostKeyFingerprint={hostKeyFingerprint}
              onHostKeyFingerprintChange={setHostKeyFingerprint}
              sshAuthType={sshAuthType}
              onSSHAuthTypeChange={setSSHAuthType}
              sshKey={sshKey}
              onSSHKeyChange={setSSHKey}
              sshPassword={sshPassword}
              onSSHPasswordChange={setSSHPassword}
              sshDeployAs={sshDeployAs}
              onSSHDeployAsChange={setSSHDeployAs}
            />
          ) : (
            <KubernetesConnectionFields
              connMethod={k8sConnMethod}
              onConnMethodChange={setK8sConnMethod}
              kubeconfig={kubeconfig}
              onKubeconfigChange={setKubeconfig}
              k8sServerUrl={k8sServerUrl}
              onServerUrlChange={setK8sServerUrl}
              k8sBearerToken={k8sBearerToken}
              onBearerTokenChange={setK8sBearerToken}
              k8sNamespace={k8sNamespace}
              onNamespaceChange={setK8sNamespace}
            />
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => { onOpenChange(false); resetForm(); }}>
            {tc('actions.cancel')}
          </Button>
          <Button
            onClick={handleCreate}
            disabled={
              isPending ||
              !envName.trim() ||
              (orchestratorType === 'docker' && connType === 'agent' && deployMethod === 'ssh' && !isSSHDeployReady)
            }
          >
            {isPending ? t('create.adding') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
