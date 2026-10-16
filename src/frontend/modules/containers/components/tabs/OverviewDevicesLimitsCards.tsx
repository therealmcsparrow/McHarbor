// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ContainerInspect } from '@core/types/docker';

export function OverviewDevicesLimitsCards({ container }: { container: ContainerInspect }) {
  const { t } = useTranslation('containers');
  const hc = container.HostConfig;
  return (
    <>
      {hc?.Devices && hc.Devices.length > 0 && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('overview.devices')}
          </h3>
          {hc.Devices.map((d) => (
            <div
              key={`${d.PathOnHost}:${d.PathInContainer}:${d.CgroupPermissions}`}
              className="flex gap-2 border-b border-border py-1.5 last:border-0"
            >
              <span className="font-mono text-xs text-foreground">{d.PathOnHost}</span>
              <span className="text-xs text-muted-foreground">→</span>
              <span className="font-mono text-xs text-foreground">{d.PathInContainer}</span>
              <span className="font-mono text-xs text-muted-foreground">({d.CgroupPermissions})</span>
            </div>
          ))}
        </div>
      )}

      {/* Ulimits */}
      {hc?.Ulimits && hc.Ulimits.length > 0 && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('overview.ulimits')}
          </h3>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-muted">
                <tr className="border-b border-border">
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('overview.ulimitName')}</th>
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('overview.ulimitSoft')}</th>
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('overview.ulimitHard')}</th>
                </tr>
              </thead>
              <tbody>
                {hc.Ulimits.map((u) => (
                  <tr key={u.Name} className="border-b border-border last:border-0">
                    <td className="px-2 py-1.5 font-mono text-foreground">{u.Name}</td>
                    <td className="px-2 py-1.5 text-foreground">{u.Soft}</td>
                    <td className="px-2 py-1.5 text-foreground">{u.Hard}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </>
  );
}
