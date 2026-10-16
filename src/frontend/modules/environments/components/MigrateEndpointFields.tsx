// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import type { ConnectionTarget } from '../hooks/useEnvironmentMigration';

export type MigrateEndpointValues = {
  socketPath: string;
  host: string;
  port: number;
  sshHost: string;
  sshPort: number;
  sshUser: string;
};

type MigrateEndpointFieldsProps = {
  target: ConnectionTarget;
  values: MigrateEndpointValues;
  onChange: (patch: Partial<MigrateEndpointValues>) => void;
};

export function MigrateEndpointFields({ target, values, onChange }: MigrateEndpointFieldsProps) {
  const { t } = useTranslation('environments');

  if (target === 'socket') {
    return (
      <div>
        <Label htmlFor="migrate-socket" className="mb-2">
          {t('create.socketPath')}
        </Label>
        <Input
          id="migrate-socket"
          value={values.socketPath}
          onChange={(changeEvent) => onChange({ socketPath: changeEvent.target.value })}
          autoComplete="off"
          spellCheck={false}
        />
      </div>
    );
  }

  if (target === 'tcp') {
    return (
      <div className="grid grid-cols-3 gap-3">
        <div className="col-span-2">
          <Label htmlFor="migrate-host" className="mb-2">
            {t('create.host')}
          </Label>
          <Input
            id="migrate-host"
            value={values.host}
            onChange={(changeEvent) => onChange({ host: changeEvent.target.value })}
            autoComplete="off"
          />
        </div>
        <div>
          <Label htmlFor="migrate-port" className="mb-2">
            {t('create.port')}
          </Label>
          <NumberInput id="migrate-port" value={values.port} onChange={(port) => onChange({ port })} />
        </div>
      </div>
    );
  }

  if (target === 'ssh') {
    return (
      <div className="space-y-3">
        <div className="grid grid-cols-3 gap-3">
          <div className="col-span-2">
            <Label htmlFor="migrate-ssh-host" className="mb-2">
              {t('create.sshHost')}
            </Label>
            <Input
              id="migrate-ssh-host"
              value={values.sshHost}
              onChange={(changeEvent) => onChange({ sshHost: changeEvent.target.value })}
              autoComplete="off"
            />
          </div>
          <div>
            <Label htmlFor="migrate-ssh-port" className="mb-2">
              {t('create.sshPort')}
            </Label>
            <NumberInput id="migrate-ssh-port" value={values.sshPort} onChange={(sshPort) => onChange({ sshPort })} />
          </div>
        </div>
        <div>
          <Label htmlFor="migrate-ssh-user" className="mb-2">
            {t('create.sshUser')}
          </Label>
          <Input
            id="migrate-ssh-user"
            value={values.sshUser}
            onChange={(changeEvent) => onChange({ sshUser: changeEvent.target.value })}
            autoComplete="off"
          />
        </div>
      </div>
    );
  }

  return null;
}
