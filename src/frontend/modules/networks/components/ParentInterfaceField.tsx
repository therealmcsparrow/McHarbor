// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconInfoCircle, IconPencil } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { useHostInterfaces, type HostInterface } from '../hooks/useNetworks';

const PARENT_KINDS = new Set(['physical', 'bond', 'vlan']);
const MANUAL_VALUE = '__manual__';

type ParentInterfaceFieldProps = {
  value: string;
  onChange: (value: string) => void;
};

function kindLabel(iface: HostInterface, t: (key: string) => string): string {
  if (iface.kind && PARENT_KINDS.has(iface.kind)) return t(`create.parentKind.${iface.kind}`);
  return t('create.parentKind.unknown');
}

function describe(iface: HostInterface, t: (key: string) => string): string {
  const parts = [kindLabel(iface, t)];
  const primary = iface.addresses?.[0];
  if (primary) parts.push(primary);
  if (iface.mtu) parts.push(`MTU ${iface.mtu}`);
  if (!iface.up && iface.source === 'agent') parts.push(t('create.parentDown'));
  if (iface.inUseBy?.length) parts.push(t('create.parentInUseShort'));
  return parts.join(' · ');
}

export function ParentInterfaceField({ value, onChange }: ParentInterfaceFieldProps) {
  const { t } = useTranslation('networks');
  const { data, isLoading } = useHostInterfaces();
  const [manualPicked, setManual] = useState(false);

  const interfaces = useMemo(() => data?.interfaces ?? [], [data?.interfaces]);
  const selected = useMemo(() => interfaces.find((iface) => iface.name === value.trim()), [interfaces, value]);

  // Nothing to pick from means the dropdown would be a dead end, so the field
  // opens as a text box instead. Resolved once the query settles rather than
  // while it is still loading, or the field would flip shape under the cursor.
  const manual = manualPicked || (!isLoading && interfaces.length === 0);

  const options = useMemo(
    () => [
      ...interfaces.map((iface) => ({
        value: iface.name,
        label: iface.name,
        description: describe(iface, t),
      })),
      { value: MANUAL_VALUE, label: t('create.parentManual'), description: t('create.parentManualHelp') },
    ],
    [interfaces, t],
  );

  const handleSelect = (next: string) => {
    if (next === MANUAL_VALUE) {
      setManual(true);
      onChange('');
      return;
    }
    onChange(next);
  };

  return (
    <div className="border-t border-border pt-4">
      <Label className="mb-1">
        {t('create.parentInterface')} <span className="text-destructive">*</span>
      </Label>

      {manual ? (
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <Input
              variant="outline"
              type="text"
              value={value}
              onChange={(e) => onChange(e.target.value)}
              placeholder={t('create.parentPlaceholder')}
            />
          </div>
          {interfaces.length > 0 && (
            <Button type="button" variant="outline" size="sm" onClick={() => setManual(false)}>
              {t('create.parentBackToList')}
            </Button>
          )}
        </div>
      ) : (
        <Select
          ariaLabel={t('create.parentInterface')}
          variant="outline"
          value={value}
          onChange={handleSelect}
          options={options}
          placeholder={isLoading ? t('create.parentLoading') : t('create.parentSelect')}
          searchable={interfaces.length > 8}
        />
      )}

      {selected?.hasDefaultRoute && (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-warning">
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {t('create.parentDefaultRoute')}
        </p>
      )}

      {selected?.inUseBy?.length ? (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-warning">
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {t('create.parentInUse', { networks: selected.inUseBy.join(', ') })}
        </p>
      ) : null}

      {selected?.source === 'agent' && !selected.up && (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-warning">
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {t('create.parentDownHelp')}
        </p>
      )}

      {manual && !isLoading && interfaces.length === 0 && (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-muted-foreground">
          <IconInfoCircle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {data?.agentAvailable
            ? t('create.parentNoneFound')
            : data?.unavailable === 'agent_unreachable'
              ? t('create.parentAgentUnreachable')
              : t('create.parentNoAgent')}
        </p>
      )}

      {manual && interfaces.length > 0 && (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-muted-foreground">
          <IconPencil className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {t('create.parentManualHelp')}
        </p>
      )}

      {!manual && !selected && <p className="mt-1 text-xs text-muted-foreground">{t('create.parentHelp')}</p>}
    </div>
  );
}
