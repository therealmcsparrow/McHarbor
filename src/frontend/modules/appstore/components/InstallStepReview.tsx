// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { PortMapping, VolumeMount } from '../types';

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
  envVars: Record<string, string>;
}

export function InstallStepReview({
  name,
  image,
  selectedEnvId,
  dockerEnvs,
  ports,
  volumes,
  envVars,
}: InstallStepReviewProps) {
  const { t } = useTranslation('common');

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
          <span className="text-muted-foreground">{t('appStore.reviewEnvironment')}</span>
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

