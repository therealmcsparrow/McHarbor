// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconBox } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import type { BulkContainerMetric } from '../hooks/useContainersBulkStats';
import { ContainerCard } from './ContainerCard';

type ContainerCardGridProps = {
  containers: ContainerInfo[];
  statsMap: Map<string, BulkContainerMetric> | undefined;
  highlightedIds?: Set<string>;
  isLoading: boolean;
  onAction: (id: string, action: string) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRecreate: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
  onClick: (c: ContainerInfo) => void;
};

const CARD_SKELETON_IDS = ['one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight'];

export function ContainerCardGrid({
  containers,
  statsMap,
  highlightedIds,
  isLoading,
  onAction,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRecreate,
  onRemove,
  onClick,
}: ContainerCardGridProps) {
  const { t } = useTranslation('containers');

  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4" aria-busy="true">
        {CARD_SKELETON_IDS.map((id) => (
          <Skeleton key={id} className="h-52 rounded-lg" aria-hidden="true" />
        ))}
      </div>
    );
  }

  if (containers.length === 0) {
    return <EmptyState icon={<IconBox className="size-5" />} title={t('noContainersFound')} />;
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {containers.map((c) => (
          <ContainerCard
            key={c.Id}
            container={c}
            stats={statsMap?.get(c.Id)}
            highlighted={highlightedIds?.has(c.Id)}
            onAction={onAction}
            onTerminal={onTerminal}
            onLogs={onLogs}
            onRename={onRename}
            onMove={onMove}
            onRecreate={onRecreate}
            onRemove={onRemove}
            onClick={onClick}
          />
        ))}
      </div>
    </div>
  );
}
