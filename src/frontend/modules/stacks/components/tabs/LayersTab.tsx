// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { api } from '@core/api/client';
import type { ImageHistoryItem } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { formatBytes } from '@resources/utils/format';
import { Spinner } from '@resources/components/ui/Spinner';
import type { StackSvc } from '@resources/hooks/useStacks';

type LayersTabProps = {
  stackName: string;
  services: StackSvc[];
};

function formatInstruction(cmd: string): string {
  return cmd.replace(/^\/bin\/sh -c #\(nop\)\s+/, '').replace(/^\/bin\/sh -c\s+/, 'RUN ');
}

export function LayersTab({ services }: LayersTabProps) {
  const { t } = useTranslation('stacks');
  const envId = useEnvironmentStore((s) => s.currentId);

  const uniqueImages = useMemo(() => {
    const seen = new Set<string>();
    return services
      .map((s) => s.image)
      .filter((img) => {
        if (seen.has(img)) return false;
        seen.add(img);
        return true;
      });
  }, [services]);

  const queries = useQueries({
    queries: uniqueImages.map((image) => ({
      queryKey: ['image-history', envId, image],
      queryFn: () =>
        api
          .get<ImageHistoryItem[]>(`/images/${encodeURIComponent(image)}/history`, envId ? { env: envId } : {})
          .then((r) => r.data ?? []),
      staleTime: 60_000,
    })),
  });

  if (uniqueImages.length === 0) {
    return <div className="py-12 text-center text-muted-foreground">{t('detail.noLayers')}</div>;
  }

  return (
    <div className="flex flex-col gap-4">
      {uniqueImages.map((image, idx) => {
        const query = queries[idx];
        return (
          <ImageLayersCard
            key={image}
            image={image}
            layers={query?.data}
            isLoading={query?.isLoading ?? true}
            isError={query?.isError ?? false}
            error={query?.error}
            t={t}
          />
        );
      })}
    </div>
  );
}

type ImageLayersCardProps = {
  image: string;
  layers: ImageHistoryItem[] | undefined;
  isLoading: boolean;
  isError: boolean;
  error?: unknown;
  t: (key: string, opts?: Record<string, string>) => string;
};

function ImageLayersCard({ image, layers, isLoading, isError, error, t }: ImageLayersCardProps) {
  const totalSize = useMemo(() => (layers ?? []).reduce((sum, l) => sum + l.Size, 0), [layers]);

  const maxLayerSize = useMemo(() => Math.max(...(layers ?? []).map((l) => l.Size), 1), [layers]);

  return (
    <div className="mc-glass overflow-hidden rounded-xl">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <span className="truncate text-sm font-medium text-foreground">{t('detail.imageLayer', { name: image })}</span>
        {layers && (
          <span className="ml-3 shrink-0 text-xs text-muted-foreground">
            {t('detail.totalSize', { size: formatBytes(totalSize) })}
          </span>
        )}
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-8">
          <Spinner size="md" />
          <span className="ml-2 text-sm text-muted-foreground">{t('detail.loadingLayers')}</span>
        </div>
      )}

      {isError && <ErrorState variant="inline" className="m-4" title={t('detail.noLayers')} error={error} />}

      {layers && !isLoading && (
        <div className="divide-y divide-border">
          {layers.map((layer, i) => {
            const pct = layer.Size > 0 ? (layer.Size / maxLayerSize) * 100 : 0;
            return (
              <div key={`${layer.Id}-${i}`} className="flex items-center gap-3 px-4 py-2">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-xs text-foreground/80">{formatInstruction(layer.CreatedBy)}</p>
                  <div className="mt-1.5 h-1.5 w-full rounded-full bg-muted">
                    <div
                      className={`h-full w-(--w) rounded-full transition-all ${
                        layer.Size > 0 ? 'bg-primary' : 'bg-muted-foreground/30'
                      }`}
                      style={cssVars({ w: layer.Size > 0 ? `${Math.max(pct, 1)}%` : '2%' })}
                    />
                  </div>
                </div>
                <span className="w-16 shrink-0 text-right text-xs text-muted-foreground tabular-nums">
                  {formatBytes(Math.max(layer.Size, 0))}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
