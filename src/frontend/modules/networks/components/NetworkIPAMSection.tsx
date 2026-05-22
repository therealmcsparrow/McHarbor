// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { IPAMConfig } from '../hooks/useNetworks';

type Driver = 'bridge' | 'host' | 'overlay' | 'macvlan' | 'ipvlan' | 'none';

interface NetworkIPAMSectionProps {
  driver: Driver;
  ipamConfigs: IPAMConfig[];
  onIpamConfigsChange: (configs: IPAMConfig[]) => void;
}

export function NetworkIPAMSection({ driver, ipamConfigs, onIpamConfigsChange }: NetworkIPAMSectionProps) {
  const { t } = useTranslation('networks');

  const updateConfig = (index: number, field: keyof IPAMConfig, value: string) => {
    const next = [...ipamConfigs];
    next[index] = { ...next[index], [field]: value };
    onIpamConfigsChange(next);
  };

  return (
    <div className="border-t border-border pt-4">
      <div className="flex items-center justify-between mb-2">
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
      {(driver === 'macvlan' || driver === 'ipvlan') && (
        <p className="text-xs text-muted-foreground mb-2">
          {t('create.ipamRequired', { driver })}
        </p>
      )}
      <div className="space-y-2">
        {ipamConfigs.map((cfg, i) => (
          <div key={`ipam-${i}`} className="flex gap-2 items-start">
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
                aria-label={t('create.removeSubnet', { defaultValue: 'Remove subnet' })}
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
