// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { NumberInput } from '@resources/components/ui/NumberInput';
import type { PortMapping } from '../types';

interface InstallStepPortsProps {
  ports: PortMapping[];
  onPortChange: (index: number, field: 'host' | 'container', value: number) => void;
}

export function InstallStepPorts({ ports, onPortChange }: InstallStepPortsProps) {
  const { t } = useTranslation('common');

  if (ports.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('appStore.noPortsConfigured')}</p>;
  }

  return (
    <div className="space-y-3">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left">
            <th className="pb-2 font-medium text-foreground">{t('appStore.hostPort')}</th>
            <th className="pb-2 font-medium text-foreground">{t('appStore.containerPort')}</th>
            <th className="pb-2 font-medium text-foreground">{t('appStore.protocol')}</th>
          </tr>
        </thead>
        <tbody>
          {ports.map((p, i) => (
            <tr key={`port-${p.container}-${p.protocol}`} className="border-b border-border/50">
              <td className="py-2 pr-2">
                <NumberInput
                  value={p.host}
                  onChange={(v) => onPortChange(i, 'host', v)}
                  min={1}
                  max={65535}
                />
              </td>
              <td className="py-2 pr-2">
                <NumberInput
                  value={p.container}
                  onChange={(v) => onPortChange(i, 'container', v)}
                  min={1}
                  max={65535}
                />
              </td>
              <td className="py-2 text-muted-foreground">{p.protocol || 'tcp'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

