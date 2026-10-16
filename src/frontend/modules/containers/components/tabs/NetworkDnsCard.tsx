// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { EditFormData } from '../../types/edit-form';

function splitCsvValue(value: string): string[] {
  return value ? value.split(',').map((item) => item.trim()) : [];
}

type NetworkDnsCardProps = {
  dns: string[];
  dnsOptions: string[];
  dnsSearch: string[];
  editing: boolean;
  onFieldChange?: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

export function NetworkDnsCard({ dns, dnsOptions, dnsSearch, editing, onFieldChange }: NetworkDnsCardProps) {
  const { t } = useTranslation('containers');
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('networkTab.dns')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsServers')}</label>
            <input
              type="text"
              value={dns.join(', ')}
              onChange={(changeEvent) => onFieldChange?.('dns', splitCsvValue(changeEvent.target.value))}
              placeholder={t('placeholders.dnsServers')}
              className="mc-control mt-1 w-full rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsSearch')}</label>
            <input
              type="text"
              value={dnsSearch.join(', ')}
              onChange={(changeEvent) => onFieldChange?.('dnsSearch', splitCsvValue(changeEvent.target.value))}
              className="mc-control mt-1 w-full rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsOptionsLabel')}</label>
            <input
              type="text"
              value={dnsOptions.join(', ')}
              onChange={(changeEvent) => onFieldChange?.('dnsOptions', splitCsvValue(changeEvent.target.value))}
              className="mc-control mt-1 w-full rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            />
          </div>
        </div>
      ) : (
        <>
          <InfoRow label={t('networkTab.dnsServers')}>{dns.length > 0 ? dns.join(', ') : '-'}</InfoRow>
          <InfoRow label={t('networkTab.dnsSearch')}>{dnsSearch.length > 0 ? dnsSearch.join(', ') : '-'}</InfoRow>
          {dnsOptions.length > 0 && <InfoRow label={t('networkTab.dnsOptionsLabel')}>{dnsOptions.join(', ')}</InfoRow>}
        </>
      )}
    </div>
  );
}
