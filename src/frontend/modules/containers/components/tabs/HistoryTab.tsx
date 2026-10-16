// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useImageHistory } from '@resources/hooks/useImages';

const SHA_PREFIX = 'sha256:';

type HistoryTabProps = {
  containerId: string;
  containerImage: string;
};

export function HistoryTab({ containerImage }: HistoryTabProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);

  // Resolve image ID by hash prefix (sha256:…) via the images list.
  const imagesList = useQuery({
    queryKey: ['images', envId],
    queryFn: () =>
      api
        .get<Array<{ Id: string; RepoTags?: string[] }>>('/images', envId ? { env: envId } : {})
        .then(assertSuccess)
        .then((images) => images ?? []),
    enabled: !!containerImage,
  });

  const matchedImageId = (() => {
    if (!containerImage || !imagesList.data) return null;
    const exact = imagesList.data.find((i) => i.Id === containerImage || i.Id.startsWith(containerImage));
    if (exact) return exact.Id;
    const byTag = imagesList.data.find((i) =>
      i.RepoTags?.some((tag) => tag === containerImage || tag === `docker.io/${containerImage}`),
    );
    return byTag?.Id ?? null;
  })();

  const imageHistory = useImageHistory(matchedImageId ?? '');
  const failure = imagesList.error ?? imageHistory.error;

  return (
    <div className="space-y-4">
      <div className="mc-glass rounded-lg p-4">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">{t('history.image')}</h3>
        </div>
        <div className="font-mono text-xs text-foreground">{containerImage}</div>
      </div>

      {imagesList.isLoading || imageHistory.isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner size="md" />
        </div>
      ) : failure ? (
        <ErrorState
          variant="card"
          error={failure}
          onRetry={() => {
            void imagesList.refetch();
            void imageHistory.refetch();
          }}
        />
      ) : !matchedImageId ? (
        <div className="mc-glass flex h-32 items-center justify-center rounded-lg text-sm text-muted-foreground">
          {t('history.imageNotFound')}
        </div>
      ) : !imageHistory.data || imageHistory.data.length === 0 ? (
        <div className="mc-glass flex h-32 items-center justify-center rounded-lg text-sm text-muted-foreground">
          {t('history.empty')}
        </div>
      ) : (
        <div className="mc-glass rounded-lg p-4">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-muted">
                <tr className="border-b border-border">
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('history.layerColumns.layer')}</th>
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('history.layerColumns.created')}</th>
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('history.layerColumns.size')}</th>
                  <th className="px-2 py-1.5 font-medium text-muted-foreground">
                    {t('history.layerColumns.instruction')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {imageHistory.data.map((layer) => (
                  <tr key={layer.Id} className="border-b border-border last:border-0 hover:bg-muted/50">
                    <td className="px-2 py-1.5 font-mono text-foreground">
                      {layer.Id.replace(SHA_PREFIX, '').slice(0, 12)}
                    </td>
                    <td className="px-2 py-1.5 text-muted-foreground">
                      {new Date(layer.Created * 1000).toISOString().replace('T', ' ').slice(0, 19)}
                    </td>
                    <td className="px-2 py-1.5 text-muted-foreground">
                      {layer.Size ? `${(layer.Size / (1024 * 1024)).toFixed(1)} MB` : '—'}
                    </td>
                    <td className="px-2 py-1.5 font-mono text-foreground">{layer.CreatedBy ?? ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
