// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import type { CreateContainerForm } from './useCreateContainerForm';

export function SetupStep({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const { ctName, hostname, setHostname, sshKeys, setSshKeys, password, setPassword, noWayIn } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.setup.title')}</h3>
      <div>
        <Label htmlFor="ct-hostname">{t('create.setup.hostname')}</Label>
        <Input id="ct-hostname" value={hostname} onChange={(ev) => setHostname(ev.target.value)} placeholder={ctName} />
      </div>
      <div>
        <Label htmlFor="ct-keys">{t('create.setup.sshKeys')}</Label>
        <Textarea id="ct-keys" rows={4} value={sshKeys} onChange={(ev) => setSshKeys(ev.target.value)} />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.setup.sshKeysHint')}</p>
      </div>
      <div>
        <Label htmlFor="ct-password">{t('create.setup.password')}</Label>
        <Input
          id="ct-password"
          type="password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          autoComplete="new-password"
        />
        {/*
          Said here rather than in a help page: an operator typing a
          password into a web form deserves to know it is hashed before
          it is stored and never written to the container's disk in the
          clear.
        */}
        <p className="mt-1 text-xs text-muted-foreground">{t('create.setup.passwordHint')}</p>
      </div>
      {noWayIn && (
        <p className="rounded-md bg-warning/10 p-3 text-sm text-foreground">{t('create.setup.noAccessWarning')}</p>
      )}
    </>
  );
}
