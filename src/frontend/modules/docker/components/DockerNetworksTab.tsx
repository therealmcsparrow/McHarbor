// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconExternalLink } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { useNetworks } from '@resources/hooks/useNetworks';

export function DockerNetworksTab() {
  const { t } = useTranslation('docker');
  const { data: networks, isLoading } = useNetworks();
  const navigate = useNavigate();

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('networks.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('networks.description')}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => navigate('/networks')}>
          <IconExternalLink className="size-4 mr-1.5" />
          {t('networks.viewAll')}
        </Button>
      </div>

      {!networks || networks.length === 0 ? (
        <div className="py-12 text-center text-sm text-muted-foreground">
          {t('networks.noNetworks')}
        </div>
      ) : (
        <div className="rounded-lg border border-border overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('networks.columnName')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('networks.columnDriver')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('networks.columnScope')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('networks.columnSubnet')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('networks.columnContainers')}</th>
              </tr>
            </thead>
            <tbody>
              {networks.map((net) => {
                const subnet = net.IPAM?.Config?.[0]?.Subnet ?? '-';
                const containerCount = net.Containers ? Object.keys(net.Containers).length : 0;

                return (
                  <tr key={net.Id} className="border-b border-border last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-2.5 font-medium text-foreground">{net.Name}</td>
                    <td className="px-4 py-2.5">
                      <Badge variant="secondary">{net.Driver}</Badge>
                    </td>
                    <td className="px-4 py-2.5 text-muted-foreground">{net.Scope}</td>
                    <td className="px-4 py-2.5 text-muted-foreground">
                      <code className="text-xs">{subnet}</code>
                    </td>
                    <td className="px-4 py-2.5 text-muted-foreground">{containerCount}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
