// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';
import { PasswordField, VaultKeysField } from './VaultCredentialFields';

export function CreateSetupStep({ form }: { form: CreateVMForm }) {
  const { t } = useTranslation('vm');
  const { hostname, setHostname, username, setUsername, sshKeys, setSshKeys, installAgent, setInstallAgent } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.setup.title')}</h3>
      <p className="text-sm text-muted-foreground">{t('create.setup.body')}</p>

      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="vm-hostname">{t('create.setup.hostname')}</Label>
          <Input
            id="vm-hostname"
            value={hostname}
            onChange={(evt) => setHostname(evt.target.value)}
            autoComplete="off"
          />
        </div>
        <div>
          <Label htmlFor="vm-username">{t('create.setup.username')}</Label>
          <Input
            id="vm-username"
            value={username}
            onChange={(evt) => setUsername(evt.target.value)}
            autoComplete="off"
          />
        </div>
      </div>

      <div>
        <Label htmlFor="vm-ssh">{t('create.setup.sshKeys')}</Label>
        <Textarea
          id="vm-ssh"
          rows={3}
          value={sshKeys}
          onChange={(evt) => setSshKeys(evt.target.value)}
          spellCheck={false}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.setup.sshKeysHint')}</p>
      </div>

      <VaultKeysField form={form} />

      <div>
        <PasswordField form={form} />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.setup.passwordHint')}</p>
        {/*
          Said next to the field, as the roadmap asks. The setup file is
          readable by anything that can read the pool, so a secret in the
          extra settings is a secret on every guest's disk.
        */}
        <p className="mt-1 text-xs text-warning">{t('create.setup.passwordWarning')}</p>
      </div>

      <div className="flex items-start justify-between gap-3">
        <div>
          <Label htmlFor="vm-agent" className="text-sm font-normal">
            {t('create.setup.installAgent')}
          </Label>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('create.setup.installAgentHint')}</p>
        </div>
        <Switch id="vm-agent" checked={installAgent} onCheckedChange={setInstallAgent} />
      </div>
    </>
  );
}
