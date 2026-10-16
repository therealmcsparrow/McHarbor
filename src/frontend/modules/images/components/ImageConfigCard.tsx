// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ImageInspect } from '@core/types/docker';
import { ImageEnvList } from './ImageEnvList';

type ImageConfigCardProps = {
  image: ImageInspect;
};

export function ImageConfigCard({ image }: ImageConfigCardProps) {
  const { t } = useTranslation('images');

  // Hoisted so each value can be handed to both the row and its `title` —
  // `spread` truncates, and an ellipsis with nothing behind it is a dead end.
  const config = image.Config;
  const cmd = config?.Cmd?.join(' ') ?? '-';
  const entrypoint = config?.Entrypoint?.join(' ') ?? '-';
  const workingDir = config?.WorkingDir || '/';
  const user = config?.User || '-';
  const stopSignal = config?.StopSignal || '-';
  const exposedPorts = Object.keys(config?.ExposedPorts ?? {}).join(', ') || '-';
  const volumes = Object.keys(config?.Volumes ?? {}).join(', ') || '-';

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('detail.config')}
      </h3>
      <InfoRow label={t('detail.fields.cmd')} variant="spread" title={cmd}>
        <span className="font-mono text-xs">{cmd}</span>
      </InfoRow>
      <InfoRow label={t('detail.fields.entrypoint')} variant="spread" title={entrypoint}>
        <span className="font-mono text-xs">{entrypoint}</span>
      </InfoRow>
      <InfoRow label={t('detail.fields.workingDir')} variant="spread" title={workingDir}>
        {workingDir}
      </InfoRow>
      <InfoRow label={t('detail.fields.user')} variant="spread" title={user}>
        {user}
      </InfoRow>
      <InfoRow label={t('detail.fields.stopSignal')} variant="spread" title={stopSignal}>
        {stopSignal}
      </InfoRow>
      <InfoRow label={t('detail.fields.exposedPorts')} variant="spread" title={exposedPorts}>
        {exposedPorts}
      </InfoRow>
      <InfoRow label={t('detail.fields.volumes')} variant="spread" title={volumes}>
        {volumes}
      </InfoRow>
      <div className="mt-3 border-t border-border pt-3">
        <span className="text-xs font-medium text-muted-foreground">{t('detail.fields.environment')}</span>
        <div className="mt-2 max-h-48 overflow-x-hidden overflow-y-auto">
          <ImageEnvList imageId={image.Id} env={image.Config?.Env} />
        </div>
      </div>
    </div>
  );
}
