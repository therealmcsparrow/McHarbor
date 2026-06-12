// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { AppNetworkSettings, PortMapping, VolumeMount } from '../types';

interface EnvironmentOption {
  id: string;
  name: string;
}

interface InstallStepReviewProps {
  name: string;
  image: string;
  selectedEnvId: string;
  dockerEnvs: EnvironmentOption[];
  ports: PortMapping[];
  volumes: VolumeMount[];
  network: AppNetworkSettings;
  envVars: Record<string, string>;
}

export function InstallStepReview({
  name,
  image,
  selectedEnvId,
  dockerEnvs,
  ports,
  volumes,
  network,
  envVars,
}: InstallStepReviewProps) {
  const { t } = useTranslation('common');
  const networkLabel = network.mode === 'existing'
    ? (network.name || t('appStore.notSet'))
    : t(`appStore.networkMode${network.mode.charAt(0).toUpperCase()}${network.mode.slice(1)}`);

  return (
    <div className="space-y-3 text-sm">
      <div className="flex justify-between">
        <span className="text-muted-foreground">{t('appStore.reviewName')}</span>
        <span className="font-medium text-foreground">{name}</span>
      </div>
      <div className="flex justify-between">
        <span className="text-muted-foreground">{t('appStore.reviewEnvironment')}</span>
        <span className="font-medium text-foreground">
          {dockerEnvs.find((e) => e.id === selectedEnvId)?.name ?? selectedEnvId}
        </span>
      </div>
      <div className="flex justify-between">
        <span className="text-muted-foreground">{t('appStore.reviewImage')}</span>
        <span className="font-mono text-xs text-foreground">{image}</span>
      </div>
      <div className="flex justify-between">
        <span className="text-muted-foreground">{t('appStore.reviewNetwork')}</span>
        <span className="font-medium text-foreground">{networkLabel}</span>
      </div>
      {network.mode === 'existing' && (network.aliases.length > 0 || network.ipv4Address || network.ipv6Address || network.macAddress) && (
        <div>
          <span className="text-muted-foreground">{t('appStore.networkSettings')}</span>
          <div className="mt-1 space-y-0.5">
            {network.aliases.length > 0 && (
              <div className="text-xs text-foreground">
                {t('appStore.networkAliases')}: {network.aliases.join(', ')}
              </div>
            )}
            {network.ipv4Address && (
              <div className="text-xs text-foreground">
                {t('appStore.ipv4Address')}: {network.ipv4Address}
              </div>
            )}
            {network.ipv6Address && (
              <div className="text-xs text-foreground">
                {t('appStore.ipv6Address')}: {network.ipv6Address}
              </div>
            )}
            {network.macAddress && (
              <div className="text-xs text-foreground">
                {t('appStore.macAddress')}: {network.macAddress}
              </div>
            )}
          </div>
        </div>
      )}
      {ports.length > 0 && (
        <div>
          <span className="text-muted-foreground">{t('appStore.reviewPorts')}</span>
          <div className="mt-1 space-y-0.5">
            {ports.map((p, i) => (
              <div key={`review-port-${p.container}-${i}`} className="text-xs text-foreground">
                {p.host}:{p.container}/{p.protocol || 'tcp'}
              </div>
            ))}
          </div>
        </div>
      )}
      {volumes.length > 0 && (
        <div>
          <span className="text-muted-foreground">{t('appStore.reviewVolumes')}</span>
          <div className="mt-1 space-y-0.5">
            {volumes.map((v, i) => (
              <div key={`review-vol-${v.container}-${i}`} className="text-xs text-foreground">
                {v.host} &rarr; {v.container}
              </div>
            ))}
          </div>
        </div>
      )}
      {Object.keys(envVars).length > 0 && (
        <div>
          <span className="text-muted-foreground">{t('appStore.reviewEnvVars')}</span>
          <div className="mt-1 space-y-0.5">
            {Object.entries(envVars).map(([k, v]) => (
              <div key={k} className="text-xs text-foreground">
                <code>{k}</code> = {v || t('appStore.empty')}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

