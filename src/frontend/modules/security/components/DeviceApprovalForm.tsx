// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconX } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { useAPIKeyGrantOptions } from '../hooks/useAPIKeys';
import { useAPIKeyForm } from '../hooks/useAPIKeyForm';
import { useApproveDevice, useDenyDevice, type DeviceRequest } from '../hooks/useDeviceSignIn';
import { APIKeyGrantFields } from './APIKeyGrantFields';

type DeviceApprovalFormProps = {
  request: DeviceRequest;
  onDecided: (approved: boolean) => void;
};

export function DeviceApprovalForm({ request, onDecided }: DeviceApprovalFormProps) {
  const { t } = useTranslation('security');
  const { data: options, isLoading } = useAPIKeyGrantOptions(true);
  const form = useAPIKeyForm(null, true);
  const [itemName, setName] = useState(`mcharbor on ${request.host || request.userCode}`);
  const approve = useApproveDevice();
  const deny = useDenyDevice();
  const busy = approve.isPending || deny.isPending;
  const failure = approve.error ?? deny.error;

  const expiryOptions = [
    { value: '', label: t('apiKeys.expirationOptions.never') },
    { value: '30', label: t('apiKeys.expirationOptions.30days') },
    { value: '90', label: t('apiKeys.expirationOptions.90days') },
    { value: '365', label: t('apiKeys.expirationOptions.1year') },
  ];

  return (
    <div className="space-y-4">
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 rounded-lg border border-border bg-muted/40 p-3 text-sm">
        <dt className="text-muted-foreground">{t('device.code')}</dt>
        <dd className="font-mono font-semibold">{request.userCode}</dd>
        <dt className="text-muted-foreground">{t('device.host')}</dt>
        <dd className="break-all">{request.host || t('device.unknown')}</dd>
        <dt className="text-muted-foreground">{t('device.osUser')}</dt>
        <dd className="break-all">{request.osUser || t('device.unknown')}</dd>
      </dl>
      <p className="text-sm text-muted-foreground">{t('device.onlyYours')}</p>

      <div className="space-y-2">
        <Label htmlFor="device-key-name">{t('apiKeys.name')}</Label>
        <Input id="device-key-name" value={itemName} maxLength={128} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="space-y-2">
        <Label>{t('apiKeys.expiresAt')}</Label>
        <Select
          value={form.state.expiry}
          onChange={form.setExpiry}
          options={expiryOptions}
          ariaLabel={t('apiKeys.expiresAt')}
        />
      </div>
      <APIKeyGrantFields
        options={options}
        loading={isLoading}
        roleIds={form.state.roleIds}
        environmentIds={form.state.environmentIds}
        selection={form.state.selection}
        scopeCount={form.scopes.length}
        onRolesChange={form.setRoleIds}
        onEnvironmentsChange={form.setEnvironmentIds}
        onSelectionChange={form.setSelection}
      />
      {!form.hasGrants && <p className="text-sm text-warning">{t('apiKeys.grantsRequired')}</p>}
      {failure && (
        <p
          role="alert"
          className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm break-words whitespace-pre-wrap text-destructive"
        >
          {failure.message}
        </p>
      )}

      <div className="flex justify-end gap-2">
        <Button
          variant="outline"
          permission={PERM.API_KEYS_MANAGE}
          disabled={busy}
          onClick={() => deny.mutate(request.userCode, { onSuccess: () => onDecided(false) })}
        >
          <IconX className="mr-1 size-4" />
          {t('device.deny')}
        </Button>
        <Button
          permission={PERM.API_KEYS_MANAGE}
          disabled={busy || !form.hasGrants || itemName.trim() === ''}
          onClick={() =>
            approve.mutate(
              {
                code: request.userCode,
                body: { name: itemName.trim(), expiresAt: form.resolveExpiresAt(), ...form.grants },
              },
              { onSuccess: () => onDecided(true) },
            )
          }
        >
          <IconCheck className="mr-1 size-4" />
          {t('device.approve')}
        </Button>
      </div>
    </div>
  );
}
