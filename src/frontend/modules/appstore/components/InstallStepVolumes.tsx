// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { VolumeMount } from '../types';

interface InstallStepVolumesProps {
  volumes: VolumeMount[];
  onVolumeChange: (index: number, field: 'host' | 'container', value: string) => void;
}

export function InstallStepVolumes({ volumes, onVolumeChange }: InstallStepVolumesProps) {
  const { t } = useTranslation('common');

  if (volumes.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('appStore.noVolumesConfigured')}</p>;
  }

  return (
    <div className="space-y-3">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left">
            <th className="pb-2 font-medium text-foreground">{t('appStore.hostPath')}</th>
            <th className="pb-2 font-medium text-foreground">{t('appStore.containerPath')}</th>
          </tr>
        </thead>
        <tbody>
          {volumes.map((v, i) => (
            <tr key={`vol-${v.container}`} className="border-b border-border/50">
              <td className="py-2 pr-2">
                <input
                  type="text"
                  value={v.host}
                  onChange={(e) => onVolumeChange(i, 'host', e.target.value)}
                  className="w-full rounded border border-border bg-background px-2 py-1 text-sm text-foreground focus:border-primary focus:outline-none"
                />
              </td>
              <td className="py-2">
                <input
                  type="text"
                  value={v.container}
                  onChange={(e) => onVolumeChange(i, 'container', e.target.value)}
                  className="w-full rounded border border-border bg-background px-2 py-1 text-sm text-foreground focus:border-primary focus:outline-none"
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

