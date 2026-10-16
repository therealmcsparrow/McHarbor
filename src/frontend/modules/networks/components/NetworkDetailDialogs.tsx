// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInfo, NetworkInfo } from '@core/types/docker';
import { isBuiltInNetwork } from '@core/utils/network';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { RemoveNetworkDialog } from './RemoveNetworkDialog';
import { Select } from '@resources/components/ui/Select';

type Translation = (key: string) => string;

type NetworkDetailDialogsProps = {
  network: NetworkInfo;
  removeId: string;
  removeDialogOpen: boolean;
  recreateDialogOpen: boolean;
  disconnectTarget: string | null;
  connectDialogOpen: boolean;
  connectPending: boolean;
  disconnectPending: boolean;
  saving: boolean;
  selectedContainer: string;
  availableContainers: ContainerInfo[];
  t: Translation;
  onRemoveDialogChange: (open: boolean) => void;
  onRecreateDialogChange: (open: boolean) => void;
  onDisconnectDialogChange: (open: boolean) => void;
  onConnectDialogChange: (open: boolean) => void;
  onSelectedContainerChange: (value: string) => void;
  onRemoved: () => void;
  onConfirmRecreate: () => void;
  onConfirmDisconnect: () => void;
  onConfirmConnect: () => void;
};

export function NetworkDetailDialogs({
  network,
  removeId,
  removeDialogOpen,
  recreateDialogOpen,
  disconnectTarget,
  connectDialogOpen,
  connectPending,
  disconnectPending,
  saving,
  selectedContainer,
  availableContainers,
  t,
  onRemoveDialogChange,
  onRecreateDialogChange,
  onDisconnectDialogChange,
  onConnectDialogChange,
  onSelectedContainerChange,
  onRemoved,
  onConfirmRecreate,
  onConfirmDisconnect,
  onConfirmConnect,
}: NetworkDetailDialogsProps) {
  const builtIn = isBuiltInNetwork(network);
  return (
    <>
      <RemoveNetworkDialog
        networkId={removeDialogOpen ? removeId : null}
        builtIn={builtIn}
        onOpenChange={onRemoveDialogChange}
        onSuccess={onRemoved}
      />

      <ConfirmDialog
        open={recreateDialogOpen}
        onOpenChange={onRecreateDialogChange}
        title={t('edit.recreateTitle')}
        description={t('edit.recreateDescription')}
        confirmLabel={t('edit.saveChanges')}
        onConfirm={onConfirmRecreate}
        loading={saving}
      />

      <ConfirmDialog
        open={disconnectTarget !== null}
        onOpenChange={onDisconnectDialogChange}
        title={t('detail.disconnectTitle')}
        description={t('detail.disconnectDescription')}
        onConfirm={onConfirmDisconnect}
        loading={disconnectPending}
      />

      <ConfirmDialog
        open={connectDialogOpen}
        onOpenChange={onConnectDialogChange}
        title={t('detail.connectTitle')}
        description={t('detail.connectDescription')}
        confirmLabel={t('detail.connect')}
        onConfirm={onConfirmConnect}
        loading={connectPending}
      >
        <div className="py-2">
          <label className="text-xs font-medium text-muted-foreground">{t('detail.selectContainer')}</label>
          <Select
            variant="outline"
            value={selectedContainer}
            onChange={onSelectedContainerChange}
            className="mt-1"
            options={[
              { value: '', label: t('detail.selectPlaceholder') },
              ...availableContainers.map((container) => ({
                value: container.Id,
                label: (container.Names?.[0] ?? '').replace(/^\//, '') || container.Id.slice(0, 12),
              })),
            ]}
          />
        </div>
      </ConfirmDialog>
    </>
  );
}
