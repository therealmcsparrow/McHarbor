// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';

type DeployMethod = 'manual' | 'ssh' | 'script';

interface DockerConnectionFieldsProps {
  connType: string;
  onConnTypeChange: (value: string) => void;
  socketPath: string;
  onSocketPathChange: (value: string) => void;
  host: string;
  onHostChange: (value: string) => void;
  port: string;
  onPortChange: (value: string) => void;
  // Agent deployment fields
  deployMethod?: DeployMethod;
  onDeployMethodChange?: (value: DeployMethod) => void;
  sshHost?: string;
  onSSHHostChange?: (value: string) => void;
  sshPort?: string;
  onSSHPortChange?: (value: string) => void;
  sshUser?: string;
  onSSHUserChange?: (value: string) => void;
  hostKeyFingerprint?: string;
  onHostKeyFingerprintChange?: (value: string) => void;
  sshAuthType?: 'key' | 'password';
  onSSHAuthTypeChange?: (value: 'key' | 'password') => void;
  sshKey?: string;
  onSSHKeyChange?: (value: string) => void;
  sshPassword?: string;
  onSSHPasswordChange?: (value: string) => void;
  sshDeployAs?: 'docker' | 'binary';
  onSSHDeployAsChange?: (value: 'docker' | 'binary') => void;
}

export function DockerConnectionFields({
  connType,
  onConnTypeChange,
  socketPath,
  onSocketPathChange,
  host,
  onHostChange,
  port,
  onPortChange,
  deployMethod = 'manual',
  onDeployMethodChange,
  sshHost = '',
  onSSHHostChange,
  sshPort = '22',
  onSSHPortChange,
  sshUser = 'root',
  onSSHUserChange,
  hostKeyFingerprint = '',
  onHostKeyFingerprintChange,
  sshAuthType = 'key',
  onSSHAuthTypeChange,
  sshKey = '',
  onSSHKeyChange,
  sshPassword = '',
  onSSHPasswordChange,
  sshDeployAs = 'docker',
  onSSHDeployAsChange,
}: DockerConnectionFieldsProps) {
  const { t } = useTranslation('environments');

  return (
    <>
      <div>
        <Label className="mb-2">{t('create.connectionType')}</Label>
        <Select
          value={connType}
          onChange={onConnTypeChange}
          options={[
            { value: 'socket', label: t('connectionTypes.socket') },
            { value: 'tcp', label: t('connectionTypes.tcp') },
            { value: 'tls', label: t('connectionTypes.tls') },
            { value: 'ssh', label: t('connectionTypes.ssh') },
            { value: 'agent', label: t('connectionTypes.agent') },
          ]}
        />
      </div>
      {connType === 'agent' ? (
        <>
          <div className="rounded-md border border-border bg-muted/50 p-3 text-sm text-muted-foreground">
            {t('create.agentDescription')}
          </div>

          <div>
            <Label className="mb-2">{t('create.deployMethod')}</Label>
            <Select
              value={deployMethod}
              onChange={(v) => onDeployMethodChange?.(v as DeployMethod)}
              options={[
                { value: 'manual', label: t('create.deployManual') },
                { value: 'ssh', label: t('create.deploySSH') },
                { value: 'script', label: t('create.deployScript') },
              ]}
            />
          </div>

          {deployMethod === 'ssh' && (
            <>
              <div>
                <Label className="mb-2">{t('create.sshHost')}</Label>
                <Input
                  type="text"
                  value={sshHost}
                  onChange={(e) => onSSHHostChange?.(e.target.value)}
                  placeholder="192.168.1.100"
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label className="mb-2">{t('create.sshPort')}</Label>
                  <NumberInput
                    value={Number(sshPort) || 22}
                    onChange={(v) => onSSHPortChange?.(String(v))}
                    min={1}
                    max={65535}
                  />
                </div>
                <div>
                  <Label className="mb-2">{t('create.sshUser')}</Label>
                  <Input
                    type="text"
                    value={sshUser}
                    onChange={(e) => onSSHUserChange?.(e.target.value)}
                    placeholder="root"
                  />
                </div>
              </div>
              <div>
                <Label className="mb-2">{t('create.hostKeyFingerprint')}</Label>
                <Input
                  type="text"
                  value={hostKeyFingerprint}
                  onChange={(e) => onHostKeyFingerprintChange?.(e.target.value)}
                  placeholder={t('create.hostKeyFingerprintPlaceholder')}
                />
                <p className="mt-1 text-xs text-muted-foreground">{t('create.hostKeyFingerprintHelp')}</p>
              </div>
              <div>
                <Label className="mb-2">{t('create.sshAuthType')}</Label>
                <Select
                  value={sshAuthType}
                  onChange={(v) => onSSHAuthTypeChange?.(v as 'key' | 'password')}
                  options={[
                    { value: 'key', label: t('create.sshAuthKey') },
                    { value: 'password', label: t('create.sshAuthPassword') },
                  ]}
                />
              </div>
              {sshAuthType === 'key' ? (
                <div>
                  <Label className="mb-2">{t('create.sshKey')}</Label>
                  <textarea
                    className="flex min-h-[100px] w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                    value={sshKey}
                    onChange={(e) => onSSHKeyChange?.(e.target.value)}
                    placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                    rows={4}
                  />
                </div>
              ) : (
                <div>
                  <Label className="mb-2">{t('create.sshPassword')}</Label>
                  <Input
                    type="password"
                    value={sshPassword}
                    onChange={(e) => onSSHPasswordChange?.(e.target.value)}
                    placeholder="••••••••"
                  />
                  <p className="mt-1 text-xs text-yellow-500">{t('create.sshPasswordWarning')}</p>
                </div>
              )}
              <div>
                <Label className="mb-2">{t('create.deployAs')}</Label>
                <Select
                  value={sshDeployAs}
                  onChange={(v) => onSSHDeployAsChange?.(v as 'docker' | 'binary')}
                  options={[
                    { value: 'docker', label: t('create.deployAsDocker') },
                    { value: 'binary', label: t('create.deployAsBinary') },
                  ]}
                />
              </div>
            </>
          )}

          {deployMethod === 'script' && (
            <div className="rounded-md border border-dashed border-border bg-muted/30 p-3">
              <p className="text-xs text-muted-foreground">
                {t('create.scriptDescription')}
              </p>
            </div>
          )}
        </>
      ) : connType === 'socket' ? (
        <div>
          <Label className="mb-2">{t('create.socketPath')}</Label>
          <Input
            type="text"
            value={socketPath}
            onChange={(e) => onSocketPathChange(e.target.value)}
            placeholder="/var/run/docker.sock"
          />
        </div>
      ) : (
        <>
          <div>
            <Label className="mb-2">{t('create.host')}</Label>
            <Input
              type="text"
              value={host}
              onChange={(e) => onHostChange(e.target.value)}
              placeholder="192.168.1.100"
            />
          </div>
          <div>
            <Label className="mb-2">{t('create.port')}</Label>
            <NumberInput
              value={Number(port) || 2375}
              onChange={(v) => onPortChange(String(v))}
              min={1}
              max={65535}
              className="w-40"
            />
          </div>
        </>
      )}
    </>
  );
}
