// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import type { ComponentProps } from 'react';
import { PluginTabPanel } from '@core/plugins/PluginTabPanel';
import { parsePluginTabId } from '@core/plugins/usePluginPageTabs';
import type { ContainerInspect } from '@core/types/docker';
import { cn } from '@resources/utils/cn';
import type { EditFormData } from '../types/edit-form';
import type { UseContainerStatsResult } from '../hooks/useContainerStats';
import type { DetailTabId } from './ContainerDetailTabs';
import { ContainerDetailRuntimePanels } from './ContainerDetailRuntimePanels';
import { ActivityStreamTab } from './tabs/ActivityStreamTab';
import { EnvironmentTab } from './tabs/EnvironmentTab';
import { LabelsTab } from './tabs/LabelsTab';
import { MountsTab } from './tabs/MountsTab';
import { NetworkTab } from './tabs/NetworkTab';
import { OverviewTab } from './tabs/OverviewTab';
import { ResourcesTab } from './tabs/ResourcesTab';
import { SecurityTab } from './tabs/SecurityTab';
import { ScansTab } from './tabs/ScansTab';

type ResourceFieldSave = NonNullable<ComponentProps<typeof ResourcesTab>['onInlineFieldSave']>;

type FieldChange = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

interface ContainerDetailTabPanelsProps {
  id: string | undefined;
  container: ContainerInspect;
  stats: UseContainerStatsResult;
  activeTab: DetailTabId;
  pluginTab: string | null;
  showBuiltIn: boolean;
  isRunning: boolean;
  protectedContainer: boolean;
  containerName: string;
  editData: EditFormData | null;
  onTabChange: (tab: DetailTabId) => void;
  onFieldChange: FieldChange;
  onInlineFieldSave: ResourceFieldSave | undefined;
}

export function ContainerDetailTabPanels({
  id,
  container,
  stats,
  activeTab,
  pluginTab,
  showBuiltIn,
  isRunning,
  protectedContainer,
  containerName,
  editData,
  onTabChange,
  onFieldChange,
  onInlineFieldSave,
}: ContainerDetailTabPanelsProps) {
  const configEditing = false;
  const parsedPluginTab = pluginTab !== null ? parsePluginTabId(pluginTab) : null;
  const builtIn = showBuiltIn ? activeTab : null;
  return (
    <div
      id="container-detail-panel"
      role="tabpanel"
      aria-labelledby={`container-tab-${activeTab}`}
      tabIndex={0}
      className={cn('flex min-h-0 flex-col pb-6', (builtIn === 'terminal' || builtIn === 'logs') && 'flex-1')}
    >
      {parsedPluginTab && (
        <PluginTabPanel
          slug={parsedPluginTab.slug}
          tabKey={parsedPluginTab.tabKey}
          context={{
            pageId: 'container',
            tabKey: parsedPluginTab.tabKey,
            entityId: id,
          }}
        />
      )}

      {builtIn === 'overview' && <OverviewTab container={container} stats={stats} onNavigate={onTabChange} />}
      {builtIn === 'environment' && (
        <EnvironmentTab
          container={container}
          editing={configEditing}
          editData={editData}
          onFieldChange={onFieldChange}
        />
      )}
      {builtIn === 'labels' && (
        <LabelsTab container={container} editing={configEditing} editData={editData} onFieldChange={onFieldChange} />
      )}
      {builtIn === 'network' && (
        <NetworkTab
          container={container}
          readOnly
          editing={configEditing}
          editData={editData}
          onFieldChange={onFieldChange}
        />
      )}
      {builtIn === 'mounts' && (
        <MountsTab
          container={container}
          onBrowseFiles={() => onTabChange('files')}
          editing={configEditing}
          editData={editData}
          onFieldChange={onFieldChange}
        />
      )}
      {builtIn === 'resources' && (
        <ResourcesTab
          container={container}
          editing={false}
          editData={editData}
          onFieldChange={onFieldChange}
          onInlineFieldSave={onInlineFieldSave}
        />
      )}
      {builtIn === 'security' && (
        <SecurityTab container={container} editing={configEditing} editData={editData} onFieldChange={onFieldChange} />
      )}
      {builtIn === 'scans' && <ScansTab container={container} />}
      {builtIn === 'activity' && (
        <ActivityStreamTab containerId={container.Id} containerImage={container.Image} isRunning={isRunning} />
      )}
      <ContainerDetailRuntimePanels
        container={container}
        activeTab={activeTab}
        builtIn={builtIn}
        isRunning={isRunning}
        protectedContainer={protectedContainer}
        containerName={containerName}
      />
    </div>
  );
}
