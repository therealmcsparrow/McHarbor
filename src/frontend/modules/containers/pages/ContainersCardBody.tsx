// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useNavigate } from 'react-router';
import type { ContainerInfo } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import { ContainerCardGrid } from '../components/ContainerCardGrid';
import type { useContainersPageDialogState } from '../hooks/useContainersPageState';

interface ContainersCardBodyProps {
  containers: ContainerInfo[];
  visibleContainers: ContainerInfo[];
  statsMap: Map<string, BulkContainerMetric> | undefined;
  highlightedIds: Set<string>;
  isLoading: boolean;
  ui: ReturnType<typeof useContainersPageDialogState>;
  onAction: (vars: { id: string; action: string }) => void;
}

export function ContainersCardBody({
  containers,
  visibleContainers,
  statsMap,
  highlightedIds,
  isLoading,
  ui,
  onAction,
}: ContainersCardBodyProps) {
  const navigate = useNavigate();
  return (
    <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
      <ContainerCardGrid
        containers={visibleContainers}
        statsMap={statsMap}
        highlightedIds={highlightedIds}
        isLoading={isLoading}
        onAction={(id, nextAction) => {
          const target = containers.find((container) => container.Id === id);
          if (target && isProtectedContainer(target)) return;
          onAction({ id, action: nextAction });
        }}
        onTerminal={ui.setTerminalTarget}
        onLogs={ui.setLogsTarget}
        onRename={ui.setRenameTarget}
        onMove={ui.setMoveTarget}
        onRecreate={ui.setRecreateTarget}
        onRemove={ui.setRemoveTarget}
        onClick={(container) => navigate(`/containers/${container.Id}`)}
      />
    </div>
  );
}
