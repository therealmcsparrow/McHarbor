// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ImageInspect } from '@core/types/docker';

type ImageConfigCardProps = {
  image: ImageInspect;
};

export function ImageConfigCard({ image }: ImageConfigCardProps) {
  const { t } = useTranslation('images');

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t('detail.config')}
      </h3>
      <InfoRow label={t('detail.fields.cmd')}>
        <span className="font-mono text-xs">{image.Config?.Cmd?.join(' ') ?? '-'}</span>
      </InfoRow>
      <InfoRow label={t('detail.fields.entrypoint')}>
        <span className="font-mono text-xs">{image.Config?.Entrypoint?.join(' ') ?? '-'}</span>
      </InfoRow>
      <InfoRow label={t('detail.fields.workingDir')}>{image.Config?.WorkingDir || '/'}</InfoRow>
      <InfoRow label={t('detail.fields.user')}>{image.Config?.User || '-'}</InfoRow>
      <InfoRow label={t('detail.fields.stopSignal')}>{image.Config?.StopSignal || '-'}</InfoRow>
      <InfoRow label={t('detail.fields.exposedPorts')}>
        {image.Config?.ExposedPorts && Object.keys(image.Config.ExposedPorts).length > 0
          ? Object.keys(image.Config.ExposedPorts).join(', ')
          : '-'}
      </InfoRow>
      <InfoRow label={t('detail.fields.volumes')}>
        {image.Config?.Volumes && Object.keys(image.Config.Volumes).length > 0
          ? Object.keys(image.Config.Volumes).join(', ')
          : '-'}
      </InfoRow>
      <div className="mt-3 border-t border-border pt-3">
        <span className="text-xs font-medium text-muted-foreground">{t('detail.fields.environment')}</span>
        <div className="mt-2 max-h-48 overflow-y-auto">
          {image.Config?.Env && image.Config.Env.length > 0 ? (
            image.Config.Env.map((env, index) => {
              const separator = env.indexOf('=');
              const key = separator > -1 ? env.slice(0, separator) : env;
              const value = separator > -1 ? env.slice(separator + 1) : '';
              return (
                <div key={`${key}-${value}-${index}`} className="flex gap-2 border-b border-border py-1.5 last:border-0">
                  <span className="shrink-0 font-mono text-xs font-medium text-foreground">{key}</span>
                  <span className="truncate font-mono text-xs text-muted-foreground">{value}</span>
                </div>
              );
            })
          ) : (
            <p className="text-sm text-muted-foreground">{t('detail.noEnvVars')}</p>
          )}
        </div>
      </div>
    </div>
  );
}
