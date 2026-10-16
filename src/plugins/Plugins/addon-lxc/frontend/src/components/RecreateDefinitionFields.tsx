// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import type { ComputeNetwork } from '../hooks/useComputeNetworks';
import type { RecreateWarning } from '../hooks/useRecreate';
import { nicAttachment } from '../utils/nicAttachment';
import type { CreateMountInput, CreateNICInput } from '../types';

export type KeyedNIC = CreateNICInput & { key: string };
export type KeyedMount = CreateMountInput & { key: string };

export function RecreateInterfaces({
  nics,
  networks,
  networksLoaded,
  onChange,
}: {
  nics: KeyedNIC[];
  networks: ComputeNetwork[];
  networksLoaded: boolean;
  onChange: (next: KeyedNIC[]) => void;
}) {
  const { t } = useTranslation('ct');

  const pickNetwork = (key: string, networkId: string) => {
    const network = networks.find((entry) => entry.id === networkId);
    onChange(
      nics.map((nic) =>
        nic.key === key && network
          ? {
              ...nic,
              networkId: network.id,
              attachKind: undefined,
              parentLink: undefined,
              linkMode: undefined,
              ...nicAttachment(network),
            }
          : nic,
      ),
    );
  };

  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium text-foreground">{t('recreate.dialog.interfaces')}</legend>
      {nics.length === 0 && <p className="text-sm text-muted-foreground">{t('recreate.dialog.noInterfaces')}</p>}
      {nics.map((nic) => {
        const nicLabel = nic.guestName || nic.key;
        const missing = networksLoaded && !networks.some((entry) => entry.id === nic.networkId);
        return (
          <div key={nic.key} className="rounded-md border border-border p-2">
            <div className="flex items-end gap-2">
              <div className="flex-1">
                <p className="mb-1 text-xs font-medium text-muted-foreground">{nicLabel}</p>
                <Select
                  value={missing ? '' : nic.networkId}
                  ariaLabel={nicLabel}
                  placeholder={t('recreate.dialog.networkPlaceholder')}
                  options={networks.map((entry) => ({ value: entry.id, label: entry.name }))}
                  onChange={(value) => pickNetwork(nic.key, value)}
                />
              </div>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('recreate.dialog.removeInterface', { name })}
                onClick={() => onChange(nics.filter((entry) => entry.key !== nic.key))}
              >
                <IconTrash className="h-4 w-4" aria-hidden="true" />
              </Button>
            </div>
            {missing && (
              <p className="mt-1 flex items-start gap-1.5 text-xs text-warning">
                <IconAlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                {t('recreate.dialog.networkMissing')}
              </p>
            )}
          </div>
        );
      })}
    </fieldset>
  );
}

export function RecreateMounts({ mounts, onChange }: { mounts: KeyedMount[]; onChange: (next: KeyedMount[]) => void }) {
  const { t } = useTranslation('ct');
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium text-foreground">{t('recreate.dialog.mounts')}</legend>
      {mounts.length === 0 && <p className="text-sm text-muted-foreground">{t('recreate.dialog.noMounts')}</p>}
      {mounts.map((mount) => (
        <div
          key={mount.key}
          className="flex items-center justify-between gap-2 rounded-md border border-border px-2 py-1.5 text-sm"
        >
          <span className="min-w-0 break-all text-foreground">
            <span className="mr-2 text-xs text-muted-foreground">{mount.kind}</span>
            <span className="font-mono text-xs">
              {mount.source} → {mount.destination}
            </span>
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('recreate.dialog.removeMount', { destination: mount.destination })}
            onClick={() => onChange(mounts.filter((entry) => entry.key !== mount.key))}
          >
            <IconTrash className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      ))}
    </fieldset>
  );
}

export function RecreateWarnings({ warnings }: { warnings: RecreateWarning[] }) {
  const { t } = useTranslation('ct');
  if (warnings.length === 0) return null;
  return (
    <ul className="space-y-1 rounded-md bg-warning/10 p-3 text-sm text-foreground">
      {warnings.map((warning) => (
        <li key={warning} className="flex items-start gap-2">
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
          {t(`recreate.warnings.${warning}`)}
        </li>
      ))}
    </ul>
  );
}

export function RecreateLimits({
  memoryMb,
  cpuMax,
  pidsMax,
  onMemoryMb,
  onCpuMax,
  onPidsMax,
}: {
  memoryMb: number;
  cpuMax: string;
  pidsMax: number;
  onMemoryMb: (value: number) => void;
  onCpuMax: (value: string) => void;
  onPidsMax: (value: number) => void;
}) {
  const { t } = useTranslation('ct');
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <div>
        <Label htmlFor="ct-recreate-memory">{t('resources.memoryMax')}</Label>
        <NumberInput id="ct-recreate-memory" min={0} value={memoryMb} onChange={onMemoryMb} />
      </div>
      <div>
        <Label htmlFor="ct-recreate-cpu">{t('resources.cpuMax')}</Label>
        <Input id="ct-recreate-cpu" value={cpuMax} onChange={(ev) => onCpuMax(ev.target.value)} />
      </div>
      <div>
        <Label htmlFor="ct-recreate-pids">{t('resources.pidsMax')}</Label>
        <NumberInput id="ct-recreate-pids" min={0} value={pidsMax} onChange={onPidsMax} />
      </div>
    </div>
  );
}
