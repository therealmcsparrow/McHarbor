// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import type { ContainerInspect } from '@core/types/docker';
import type { DetailTabId } from './ContainerDetailTabs';
import { BackupsTab } from './tabs/BackupsTab';
import { BackupSettingsTab } from './tabs/BackupSettingsTab';
import { BackupSchedulesTab } from './tabs/BackupSchedulesTab';
import { FilesTab } from './tabs/FilesTab';
import { LogsTab } from './tabs/LogsTab';
import { ProcessesTab } from './tabs/ProcessesTab';
import { TerminalTab } from './tabs/TerminalTab';
import { ChangesTab } from './tabs/ChangesTab';
import { DiffTab } from './tabs/DiffTab';
import { HistoryTab } from './tabs/HistoryTab';
import { InspectTab } from './tabs/InspectTab';
import { ServicesTab } from './tabs/ServicesTab';
import { UpdatesTab } from './tabs/UpdatesTab';
import { ExecTab } from './tabs/ExecTab';

interface ContainerDetailRuntimePanelsProps {
  container: ContainerInspect;
  activeTab: DetailTabId;
  builtIn: DetailTabId | null;
  isRunning: boolean;
  protectedContainer: boolean;
  containerName: string;
}

export function ContainerDetailRuntimePanels({
  container,
  activeTab,
  builtIn,
  isRunning,
  protectedContainer,
  containerName,
}: ContainerDetailRuntimePanelsProps) {
  return (
    <>
      <div className={activeTab !== 'logs' ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}>
        <LogsTab container={container} isRunning={isRunning} active={builtIn === 'logs'} />
      </div>
      <div className={activeTab !== 'terminal' ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}>
        <TerminalTab containerId={container.Id} isRunning={isRunning} active={builtIn === 'terminal'} />
      </div>
      {builtIn === 'processes' && <ProcessesTab containerId={container.Id} isRunning={isRunning} />}
      {builtIn === 'files' && (
        <FilesTab
          containerId={container.Id}
          isRunning={isRunning}
          mounts={container.Mounts ?? []}
          readOnly={protectedContainer}
        />
      )}
      {builtIn === 'backup-settings' && <BackupSettingsTab containerId={container.Id} containerName={containerName} />}
      {builtIn === 'backup-schedules' && <BackupSchedulesTab containerId={container.Id} />}
      {builtIn === 'backups' && <BackupsTab containerId={container.Id} containerName={containerName} />}
      {builtIn === 'inspect' && <InspectTab containerId={container.Id} />}
      {builtIn === 'updates' && <UpdatesTab containerId={container.Id} containerImage={container.Image} />}
      {builtIn === 'services' && <ServicesTab containerId={container.Id} isRunning={isRunning} />}
      {builtIn === 'exec' && <ExecTab containerId={container.Id} isRunning={isRunning} />}
      {builtIn === 'history' && <HistoryTab containerId={container.Id} containerImage={container.Image} />}
      {builtIn === 'changes' && <ChangesTab containerId={container.Id} />}
      {builtIn === 'diff' && <DiffTab containerId={container.Id} isRunning={isRunning} />}
    </>
  );
}
