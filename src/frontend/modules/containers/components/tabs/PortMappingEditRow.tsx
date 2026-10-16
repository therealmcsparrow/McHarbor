// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import type { PortMappingEntry } from '../../types/edit-form';

// TCP / UDP are protocol identifiers, not prose — see .instructions/donottranslate.md.
const PROTOCOL_OPTIONS = [
  { value: 'tcp', label: 'TCP' },
  { value: 'udp', label: 'UDP' },
];

const FIELD_CLASS =
  'mc-control min-h-11 rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:min-h-0';

type PortMappingEditRowProps = {
  mapping: PortMappingEntry;
  index: number;
  onChange: (index: number, field: keyof PortMappingEntry, value: string) => void;
  onRemove: (index: number) => void;
};

// One editable port-mapping row on the container network tab: host port,
// container port, host IP, protocol and the remove control.
export function PortMappingEditRow({ mapping, index, onChange, onRemove }: PortMappingEditRowProps) {
  const { t } = useTranslation('containers');

  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_100px_80px_auto]">
      <input
        type="text"
        value={mapping.hostPort}
        onChange={(changeEvent) => onChange(index, 'hostPort', changeEvent.target.value)}
        placeholder="8080"
        aria-label={t('networkTab.mappingFieldLabel', {
          field: t('networkTab.hostPort'),
          number: index + 1,
        })}
        className={FIELD_CLASS}
      />
      <input
        type="text"
        value={mapping.containerPort}
        onChange={(changeEvent) => onChange(index, 'containerPort', changeEvent.target.value)}
        placeholder="80"
        aria-label={t('networkTab.mappingFieldLabel', {
          field: t('networkTab.containerPort'),
          number: index + 1,
        })}
        className={FIELD_CLASS}
      />
      <input
        type="text"
        value={mapping.hostIp}
        onChange={(changeEvent) => onChange(index, 'hostIp', changeEvent.target.value)}
        placeholder="0.0.0.0"
        aria-label={t('networkTab.mappingFieldLabel', {
          field: t('networkTab.hostIp'),
          number: index + 1,
        })}
        className={FIELD_CLASS}
      />
      <Select
        value={mapping.protocol}
        onChange={(next) => onChange(index, 'protocol', next)}
        options={PROTOCOL_OPTIONS}
        searchable={false}
        ariaLabel={t('networkTab.mappingFieldLabel', {
          field: t('networkTab.protocol'),
          number: index + 1,
        })}
        variant="outline"
        className="sm: min-h-11 w-24 text-xs sm:min-h-0"
      />
      <Button
        variant="ghost"
        size="icon"
        onClick={() => onRemove(index)}
        aria-label={t('network.removePort')}
        className="size-11 text-muted-foreground hover:text-destructive sm:size-9"
      >
        <IconTrash className="size-3.5" />
      </Button>
    </div>
  );
}
