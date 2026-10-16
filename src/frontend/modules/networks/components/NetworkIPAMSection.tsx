// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import type { IPAMConfig } from '../hooks/useNetworks';

type Driver = 'bridge' | 'host' | 'overlay' | 'macvlan' | 'ipvlan' | 'none';

interface NetworkIPAMSectionProps {
  driver: Driver;
  ipamConfigs: IPAMConfig[];
  onIpamConfigsChange: (configs: IPAMConfig[]) => void;
  enableIPv6: boolean;
  onEnableIPv6Change: (value: boolean) => void;
}

export function NetworkIPAMSection({
  driver,
  ipamConfigs,
  onIpamConfigsChange,
  enableIPv6,
  onEnableIPv6Change,
}: NetworkIPAMSectionProps) {
  const { t } = useTranslation('networks');
  // Rows are removable, so an index key would hand a removed row's DOM node —
  // and the caret sitting in it — to the row that took its place.
  const rowKeys = useStableListKeys(ipamConfigs);

  const updateConfig = (index: number, field: keyof IPAMConfig, value: string) => {
    const next = [...ipamConfigs];
    next[index] = { ...next[index], [field]: value };
    onIpamConfigsChange(next);
  };

  return (
    <div className="border-t border-border pt-4">
      <div className="mb-2 flex items-center justify-between">
        <Label>{t('create.ipam')}</Label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => onIpamConfigsChange([...ipamConfigs, { Subnet: '', Gateway: '', IPRange: '' }])}
        >
          <IconPlus className="h-3.5 w-3.5" /> {t('create.addSubnet')}
        </Button>
      </div>
      <div className="mb-2 flex items-start justify-between gap-3 rounded-md border border-border px-3 py-2">
        <div className="min-w-0">
          <p className="text-sm">{t('create.ipv6')}</p>
          <p className="text-xs text-muted-foreground">{t('create.ipv6Hint')}</p>
        </div>
        <Switch checked={enableIPv6} aria-label={t('create.ipv6')} onCheckedChange={onEnableIPv6Change} />
      </div>
      {(driver === 'macvlan' || driver === 'ipvlan') && (
        <p className="mb-2 text-xs text-muted-foreground">{t('create.ipamRequired', { driver })}</p>
      )}
      <div className="space-y-2">
        {ipamConfigs.map((cfg, i) => (
          <div key={rowKeys[i]} className="flex items-start gap-2">
            <Input
              variant="outline"
              type="text"
              value={cfg.Subnet ?? ''}
              onChange={(e) => updateConfig(i, 'Subnet', e.target.value)}
              placeholder={t('create.subnetPlaceholder')}
              className="flex-1"
            />
            <Input
              variant="outline"
              type="text"
              value={cfg.Gateway ?? ''}
              onChange={(e) => updateConfig(i, 'Gateway', e.target.value)}
              placeholder={t('create.gatewayPlaceholder')}
              className="flex-1"
            />
            <Input
              variant="outline"
              type="text"
              value={cfg.IPRange ?? ''}
              onChange={(e) => updateConfig(i, 'IPRange', e.target.value)}
              placeholder={t('create.ipRangePlaceholder')}
              className="flex-1"
            />
            {ipamConfigs.length > 1 && (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('create.removeSubnet')}
                onClick={() => onIpamConfigsChange(ipamConfigs.filter((_, j) => j !== i))}
              >
                <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
              </Button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
