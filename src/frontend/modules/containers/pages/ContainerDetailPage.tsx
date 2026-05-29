// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { useContainerStackLink } from '@resources/hooks/useStackLinks';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useContainer, useContainerAction } from '../hooks/useContainers';
import { useContainerEdit } from '../hooks/useContainerEdit';
import { ContainerDetailDialogs } from '../components/ContainerDetailDialogs';
import { ContainerDetailTabs, type DetailTabId } from '../components/ContainerDetailTabs';
import { EnvironmentTab } from '../components/tabs/EnvironmentTab';
import { FilesTab } from '../components/tabs/FilesTab';
import { LabelsTab } from '../components/tabs/LabelsTab';
import { LogsTab } from '../components/tabs/LogsTab';
import { MountsTab } from '../components/tabs/MountsTab';
import { NetworkTab } from '../components/tabs/NetworkTab';
import { OverviewTab } from '../components/tabs/OverviewTab';
import { ProcessesTab } from '../components/tabs/ProcessesTab';
import { ResourcesTab } from '../components/tabs/ResourcesTab';
import { SecurityTab } from '../components/tabs/SecurityTab';
import { TerminalTab } from '../components/tabs/TerminalTab';
import { SaveBar } from '../components/SaveBar';
import { ContainerDetailHeader, getInspectWebUrl } from './ContainerDetailHeader';
import { isProtectedContainer } from '@core/utils/protection';

export default function ContainerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const { data: container, isLoading } = useContainer(id ?? '');
  const { data: stackLink } = useContainerStackLink(id);
  const action = useContainerAction();
  const edit = useContainerEdit(container);
  const [confirmKill, setConfirmKill] = useState(false);
  const [recreateConfirmOpen, setRecreateConfirmOpen] = useState(false);
  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [takeOverOpen, setTakeOverOpen] = useState(false);
  const [relinkOpen, setRelinkOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<DetailTabId>('overview');
  const setHeaderActive = useHeaderSlot((store) => store.setActive);

  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  const handleSave = useCallback(() => {
    if (edit.changes.hasConfigChanges) {
      setRecreateConfirmOpen(true);
      return;
    }
    edit.save();
  }, [edit]);

  if (isLoading) {
    return <div className="flex h-64 items-center justify-center"><Spinner size="lg" /></div>;
  }

  if (!container) {
    return <div className="py-12 text-center text-muted-foreground">{t('containerNotFound')}</div>;
  }

  const name = (container.Name ?? '').replace(/^\//, '');
  const isRunning = (container.State?.Status ?? 'unknown') === 'running';
  const webURL = isRunning ? getInspectWebUrl(container.NetworkSettings?.Ports) : null;
  const linkedStackName = stackLink?.stackName ?? container.Config?.Labels?.['com.docker.compose.project'] ?? null;
  const isComposeManaged = !!container.Config?.Labels?.['com.docker.compose.project'];
  const locked = isProtectedContainer(container);

  return (
    <div className="flex h-full flex-col gap-0">
      {document.getElementById('header-slot') &&
        createPortal(
          <ContainerDetailHeader
            container={container}
            isRunning={isRunning}
            name={name}
            webUrl={webURL}
            editing={edit.editing}
            onEdit={edit.startEditing}
            onSave={handleSave}
            onCancelEdit={edit.cancelEditing}
            saving={edit.isSaving}
            stackName={linkedStackName}
            onAction={(nextAction) => action.mutate({ id: container.Id, action: nextAction })}
            onKill={() => setConfirmKill(true)}
            onRemove={() => setRemoveDialogOpen(true)}
            onTakeOver={() => setTakeOverOpen(true)}
            onRelink={() => setRelinkOpen(true)}
          />,
          document.getElementById('header-slot')!,
        )}

      <div className="flex h-full flex-col overflow-hidden">
        <ContainerDetailTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          labelFor={(tab) => t(`detail.${tab}`)}
        />
        <div className={`flex min-h-0 flex-1 flex-col p-5 ${activeTab === 'terminal' || activeTab === 'logs' ? 'overflow-hidden' : 'overflow-y-auto'}`}>
          {activeTab === 'overview' && <OverviewTab container={container} />}
          {activeTab === 'environment' && <EnvironmentTab container={container} editing={edit.editing} editData={edit.editData} onFieldChange={edit.onFieldChange} />}
          {activeTab === 'labels' && <LabelsTab container={container} editing={edit.editing} editData={edit.editData} onFieldChange={edit.onFieldChange} />}
          {activeTab === 'network' && <NetworkTab container={container} editing={edit.editing} editData={edit.editData} onFieldChange={edit.onFieldChange} />}
          {activeTab === 'mounts' && <MountsTab container={container} />}
          {activeTab === 'resources' && <ResourcesTab container={container} editing={edit.editing} editData={edit.editData} onFieldChange={edit.onFieldChange} />}
          {activeTab === 'security' && <SecurityTab container={container} editing={edit.editing} editData={edit.editData} onFieldChange={edit.onFieldChange} />}
          <div className={activeTab !== 'logs' ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}><LogsTab containerId={container.Id} isRunning={isRunning} /></div>
          <div className={activeTab !== 'terminal' ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}><TerminalTab containerId={container.Id} isRunning={isRunning} active={activeTab === 'terminal'} /></div>
          {activeTab === 'processes' && <ProcessesTab containerId={container.Id} isRunning={isRunning} />}
          {activeTab === 'files' && <FilesTab containerId={container.Id} isRunning={isRunning} mounts={container.Mounts ?? []} readOnly={locked} />}
        </div>

        {edit.editing && (
          <SaveBar
            changes={edit.changes}
            onSave={handleSave}
            onCancel={edit.cancelEditing}
            isSaving={edit.isSaving}
            isComposeManaged={isComposeManaged}
          />
        )}
      </div>

      <ContainerDetailDialogs
        container={container}
        containerName={name}
        confirmKill={confirmKill}
        recreateConfirmOpen={recreateConfirmOpen}
        removeDialogOpen={removeDialogOpen}
        takeOverOpen={takeOverOpen}
        relinkOpen={relinkOpen}
        linkedStackName={linkedStackName}
        actionPending={action.isPending}
        editSaving={edit.isSaving}
        changedFields={edit.changes.changedConfigFields}
        onConfirmKillChange={(open) => setConfirmKill(open)}
        onRecreateConfirmChange={setRecreateConfirmOpen}
        onRemoveDialogChange={setRemoveDialogOpen}
        onTakeOverChange={setTakeOverOpen}
        onRelinkChange={setRelinkOpen}
        onKill={() => {
          action.mutate({ id: container.Id, action: 'kill' });
          setConfirmKill(false);
        }}
        onConfirmRecreate={() => {
          setRecreateConfirmOpen(false);
          edit.save();
        }}
        onRemoveSuccess={() => navigate('/containers')}
      />
    </div>
  );
}
