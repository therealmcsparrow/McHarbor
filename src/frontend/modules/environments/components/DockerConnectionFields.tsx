// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { SshAuthFields } from './SshAuthFields';

type DeployMethod = 'manual' | 'ssh';

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

  const connectionTypeOptions = [
    { value: 'socket', label: t('connectionTypes.socket') },
    { value: 'agent', label: t('connectionTypes.agent') },
  ];
  if (connType !== 'socket' && connType !== 'agent') {
    connectionTypeOptions.push({ value: connType, label: t(`connectionTypes.${connType}`) });
  }

  return (
    <>
      <div>
        <Label className="mb-2">{t('create.connectionType')}</Label>
        <Select value={connType} onChange={onConnTypeChange} options={connectionTypeOptions} />
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
                    placeholder={t('placeholders.sshUser')}
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
              <SshAuthFields
                sshAuthType={sshAuthType}
                onSSHAuthTypeChange={onSSHAuthTypeChange}
                sshKey={sshKey}
                onSSHKeyChange={onSSHKeyChange}
                sshPassword={sshPassword}
                onSSHPasswordChange={onSSHPasswordChange}
                sshDeployAs={sshDeployAs}
                onSSHDeployAsChange={onSSHDeployAsChange}
              />
            </>
          )}
        </>
      ) : connType === 'socket' ? (
        <div>
          <Label className="mb-2">{t('create.socketPath')}</Label>
          <Input
            type="text"
            value={socketPath}
            onChange={(e) => onSocketPathChange(e.target.value)}
            placeholder={t('create.socketPathPlaceholder')}
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
