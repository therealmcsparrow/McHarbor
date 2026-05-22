// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createPortal } from 'react-dom';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { NetworkDetailDialogs } from '../components/NetworkDetailDialogs';
import {
  NetworkConnectedContainersSection,
  NetworkDetailPanels,
} from '../components/NetworkDetailPanels';
import { useNetworkDetailState } from '../hooks/useNetworkDetailState';
import { NetworkDetailHeader } from './NetworkDetailHeader';

export default function NetworkDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('networks');
  const state = useNetworkDetailState(id, t);

  if (state.isLoading) {
    return <div className="flex h-64 items-center justify-center"><Spinner size="lg" /></div>;
  }

  if (!state.network) {
    return <div className="py-12 text-center text-muted-foreground">{t('detail.notFound')}</div>;
  }

  return (
    <div className="flex h-full flex-col gap-0">
      {document.getElementById('header-slot') &&
        createPortal(
          <NetworkDetailHeader
            name={state.network.Name}
            id={state.network.Id}
            driver={state.network.Driver}
            scope={state.network.Scope}
            editing={state.editing}
            saving={state.saving}
            onEdit={state.startEditing}
            onSave={() => state.setRecreateDialogOpen(true)}
            onCancelEdit={state.cancelEditing}
            onRemove={() => state.setRemoveDialogOpen(true)}
          />,
          document.getElementById('header-slot')!,
        )}

      <div className="flex h-full flex-col overflow-hidden">
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <NetworkDetailPanels
              network={state.network}
              editData={state.editData}
              editing={state.editing}
              t={t}
              onUpdateEdit={state.updateEdit}
              onUpdateIPAMEntry={state.updateIPAMEntry}
              onAddIPAMEntry={state.addIPAMEntry}
              onRemoveIPAMEntry={state.removeIPAMEntry}
            />
          </div>

          <NetworkConnectedContainersSection
            editing={state.editing}
            connectedContainers={state.connectedContainers}
            t={t}
            onOpenConnect={() => {
              state.setSelectedContainer('');
              state.setConnectDialogOpen(true);
            }}
            onDisconnect={state.setDisconnectTarget}
            onContainerOpen={(containerID) => navigate(`/containers/${containerID}`)}
          />
        </div>
      </div>

      <NetworkDetailDialogs
        removeDialogOpen={state.removeDialogOpen}
        recreateDialogOpen={state.recreateDialogOpen}
        disconnectTarget={state.disconnectTarget}
        connectDialogOpen={state.connectDialogOpen}
        connectPending={state.connectNetwork.isPending}
        disconnectPending={state.disconnectNetwork.isPending}
        removePending={state.removeNetwork.isPending}
        saving={state.saving}
        selectedContainer={state.selectedContainer}
        availableContainers={state.availableContainers}
        t={t}
        onRemoveDialogChange={(open) => !open && state.setRemoveDialogOpen(false)}
        onRecreateDialogChange={(open) => !open && state.setRecreateDialogOpen(false)}
        onDisconnectDialogChange={(open) => !open && state.setDisconnectTarget(null)}
        onConnectDialogChange={(open) => !open && state.setConnectDialogOpen(false)}
        onSelectedContainerChange={state.setSelectedContainer}
        onConfirmRemove={() => {
          state.removeNetwork.mutate(id, {
            onSuccess: () => navigate('/networks'),
          });
          state.setRemoveDialogOpen(false);
        }}
        onConfirmRecreate={state.confirmSave}
        onConfirmDisconnect={() => {
          if (state.disconnectTarget) {
            state.disconnectNetwork.mutate({ networkId: id, container: state.disconnectTarget });
          }
          state.setDisconnectTarget(null);
        }}
        onConfirmConnect={() => {
          if (state.selectedContainer) {
            state.connectNetwork.mutate({ networkId: id, container: state.selectedContainer });
          }
          state.setConnectDialogOpen(false);
        }}
      />
    </div>
  );
}
