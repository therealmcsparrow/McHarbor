// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { Input } from '@resources/components/ui/Input';

export type NetworkEndpointSettingsDraft = {
  aliases: string;
  ipv4Address: string;
  ipv6Address: string;
  macAddress: string;
};

type NetworkEndpointSettingsFieldsProps = {
  draft: NetworkEndpointSettingsDraft;
  disabled?: boolean;
  onChange: (patch: Partial<NetworkEndpointSettingsDraft>) => void;
  t: TFunction<'containers'>;
};

export function emptyNetworkEndpointSettingsDraft(): NetworkEndpointSettingsDraft {
  return {
    aliases: '',
    ipv4Address: '',
    ipv6Address: '',
    macAddress: '',
  };
}

export function splitNetworkAliases(value: string): string[] {
  return value
    .split(',')
    .map((alias) => alias.trim())
    .filter(Boolean);
}

export function NetworkEndpointSettingsFields({
  draft,
  disabled,
  onChange,
  t,
}: NetworkEndpointSettingsFieldsProps) {
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <label className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">{t('networkTab.aliases')}</span>
        <Input
          variant="outline"
          value={draft.aliases}
          disabled={disabled}
          onChange={(event) => onChange({ aliases: event.target.value })}
          placeholder="app, api"
        />
      </label>
      <label className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">{t('networkTab.ipv4Address')}</span>
        <Input
          variant="outline"
          value={draft.ipv4Address}
          disabled={disabled}
          onChange={(event) => onChange({ ipv4Address: event.target.value })}
          placeholder="172.20.0.10"
        />
      </label>
      <label className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">{t('networkTab.ipv6Address')}</span>
        <Input
          variant="outline"
          value={draft.ipv6Address}
          disabled={disabled}
          onChange={(event) => onChange({ ipv6Address: event.target.value })}
          placeholder="fd00::10"
        />
      </label>
      <label className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">{t('networkTab.macAddress')}</span>
        <Input
          variant="outline"
          value={draft.macAddress}
          disabled={disabled}
          onChange={(event) => onChange({ macAddress: event.target.value })}
          placeholder="02:42:ac:14:00:0a"
        />
      </label>
    </div>
  );
}
