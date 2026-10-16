// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// NetworkSection renders the network picker for the create-container
// dialog: existing user-defined networks, the built-in bridge/host/
// none defaults, and an inline "create new network" mini-form that
// supports every Docker driver.
//
// Extracted from `CreateContainerDialog.tsx` so that file can stay
// under the project 200-line cap. The component is self-contained:
// all of its state is local and all of its props are passed by the
// parent dialog.

import { useTranslation } from 'react-i18next';
import { IconNetwork } from '@tabler/icons-react';
import { Select, type SelectOption } from '@resources/components/ui/Select';
import { Badge } from '@resources/components/ui/Badge';
import { Caption } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { FieldLabel, FieldHelp, Section } from '@resources/components/ui/Form';
import { cn } from '@resources/utils/cn';
import { networkDriverClasses } from '@resources/utils/statusPalette';
import type { NetworkInfo } from '@core/types/docker';
import { DRIVER_OPTIONS } from '@resources/constants/network-drivers';
import type { NetworkChoice } from '../hooks/useCreateContainer';
import { CreateContainerNewNetworkForm } from './CreateContainerNewNetworkForm';
import { useNewNetworkDraft } from './useNewNetworkDraft';

type NetworkSectionProps = {
  network: NetworkChoice;
  setNetwork: (v: NetworkChoice) => void;
  existingNetworks: NetworkInfo[];
  onNetworkCreated: (network: NetworkInfo) => void;
};

export function NetworkSection({ network, setNetwork, existingNetworks, onNetworkCreated }: NetworkSectionProps) {
  const { t } = useTranslation('containers');
  const draft = useNewNetworkDraft(onNetworkCreated);

  // Derive the mode from the current network choice. If the current
  // choice was made by the Create-new flow earlier (kind = 'custom'),
  // we still treat it as 'create' so the inline form stays visible
  // until the user picks something else.
  const mode: 'driver' | 'existing' | 'create' =
    network.kind === 'existing'
      ? 'existing'
      : network.kind === 'custom' || network.kind === 'new'
        ? 'create'
        : 'driver';

  function driverBadge(driver: string): string {
    return networkDriverClasses(driver);
  }

  const modeOptions: SelectOption[] = [
    { value: 'driver', label: t('create.form.networkModeDriver') },
    { value: 'existing', label: t('create.form.networkModeExisting') },
    { value: 'create', label: t('create.form.networkModeCreate') },
  ];

  const driverOptions: SelectOption[] = DRIVER_OPTIONS.map((opt) => ({
    value: opt.value,
    label: opt.label,
    description: ['bridge', 'host', 'none'].includes(opt.value) ? undefined : t('create.form.networkNewHelp'),
  }));

  const userDefinedNetworks = existingNetworks.filter(
    (n) => !['bridge', 'host', 'none'].includes(n.Name.toLowerCase()),
  );

  const existingOptions: SelectOption[] = userDefinedNetworks.map((n) => ({
    value: n.Name,
    label: n.Name,
    description: n.Driver,
  }));

  function handleModeChange(next: 'driver' | 'existing' | 'create') {
    if (next === 'driver') {
      setNetwork({ kind: 'default', driver: 'bridge' });
    } else if (next === 'existing') {
      const first = userDefinedNetworks[0];
      setNetwork(first ? { kind: 'existing', name: first.Name } : { kind: 'default', driver: 'bridge' });
    } else {
      // create — keep any draft the user already typed, don't override
      setNetwork({ kind: 'custom', name: '__draft__', driver: draft.newDriver });
    }
  }

  return (
    <Section
      icon={<IconNetwork className="size-3.5" />}
      title={t('create.form.network')}
      help={t('create.form.networkHelp')}
    >
      <Box className="space-y-3">
        {/* First pulldown: pick a source */}
        <Box className="space-y-1.5">
          <FieldLabel>{t('create.form.networkMode')}</FieldLabel>
          <Select
            value={mode}
            onChange={(v) => handleModeChange(v as 'driver' | 'existing' | 'create')}
            options={modeOptions}
            searchable={false}
          />
          <FieldHelp>{t('create.form.networkModeHelp')}</FieldHelp>
        </Box>

        {/* Second pulldown: depends on the first */}
        {mode === 'driver' && (
          <Box className="space-y-1.5">
            <FieldLabel>{t('create.form.networkSelectDriver')}</FieldLabel>
            <Select
              value={network.kind === 'default' ? network.driver : 'bridge'}
              onChange={(v) => setNetwork({ kind: 'default', driver: v as 'bridge' | 'host' | 'none' })}
              options={driverOptions.filter((d) => ['bridge', 'host', 'none'].includes(d.value))}
              searchable={false}
            />
            <FieldHelp>{t('create.form.networkSelectDriverHelp')}</FieldHelp>
          </Box>
        )}

        {mode === 'existing' && (
          <Box className="space-y-1.5">
            <FieldLabel>{t('create.form.networkSelectExisting')}</FieldLabel>
            {userDefinedNetworks.length > 0 ? (
              <Select
                value={network.kind === 'existing' ? network.name : (userDefinedNetworks[0]?.Name ?? '')}
                onChange={(v) => setNetwork({ kind: 'existing', name: v })}
                options={existingOptions}
                searchable={false}
              />
            ) : (
              <Caption className="rounded-md border border-dashed border-border bg-background/40 px-3 py-2 text-[11px] text-muted-foreground italic">
                {t('create.form.networkSelectEmpty')}
              </Caption>
            )}
            <FieldHelp>{t('create.form.networkSelectExistingHelp')}</FieldHelp>
            {network.kind === 'existing' && network.name && (
              <Flex gap={2} className="flex-wrap">
                {userDefinedNetworks
                  .filter((n) => n.Name === network.name)
                  .map((n) => (
                    <Badge
                      key={n.Id ?? n.Name}
                      variant="outline"
                      className={cn('text-[10px] uppercase', driverBadge(n.Driver))}
                    >
                      {n.Driver}
                    </Badge>
                  ))}
              </Flex>
            )}
          </Box>
        )}

        {mode === 'create' && <CreateContainerNewNetworkForm draft={draft} driverOptions={driverOptions} />}
      </Box>
    </Section>
  );
}
