// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Binds the network detail page's state object to NetworkDetailDialogs.
// Split out of NetworkDetailPage.tsx unchanged — it is prop wiring only, so
// keeping it beside the page rather than inside it costs nothing and gives
// the page back its shape.

import type { NetworkInfo } from '@core/types/docker';
import { NetworkDetailDialogs } from '../components/NetworkDetailDialogs';
import type { useNetworkDetailState } from '../hooks/useNetworkDetailState';

type NetworkDetailState = ReturnType<typeof useNetworkDetailState>;

type NetworkDetailPageDialogsProps = {
  network: NetworkInfo;
  id: string;
  state: NetworkDetailState;
  t: (key: string) => string;
  onRemoved: () => void;
};

export function NetworkDetailPageDialogs({ network, id, state, t, onRemoved }: NetworkDetailPageDialogsProps) {
  return (
    <NetworkDetailDialogs
      network={network}
      removeId={id}
      removeDialogOpen={state.removeDialogOpen}
      recreateDialogOpen={state.recreateDialogOpen}
      disconnectTarget={state.disconnectTarget}
      connectDialogOpen={state.connectDialogOpen}
      connectPending={state.connectNetwork.isPending}
      disconnectPending={state.disconnectNetwork.isPending}
      saving={state.saving}
      selectedContainer={state.selectedContainer}
      availableContainers={state.availableContainers}
      t={t}
      onRemoveDialogChange={(next) => !next && state.setRemoveDialogOpen(false)}
      onRecreateDialogChange={(next) => !next && state.setRecreateDialogOpen(false)}
      onDisconnectDialogChange={(next) => !next && state.setDisconnectTarget(null)}
      onConnectDialogChange={(next) => !next && state.setConnectDialogOpen(false)}
      onSelectedContainerChange={state.setSelectedContainer}
      onRemoved={onRemoved}
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
  );
}
