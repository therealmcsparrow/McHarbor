// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { NetworkInfo } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { Dialog, DialogBody, DialogContent } from '@resources/components/ui/Dialog';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import type { MoveNetworkConfig, MoveVolumeConfig } from '../hooks/useContainers';
import { useMoveContainerPlan, useMoveContainerStream } from '../hooks/useContainers';
import { MoveContainerDialogFooter } from './MoveContainerDialogFooter';
import { MoveContainerDialogHeader } from './MoveContainerDialogHeader';
import { moveNetworkConfigsFromPlan } from './MoveNetworkSettings';
import { moveVolumeConfigsFromPlan } from './MoveVolumeSettings';
import { MoveContainerSetup } from './MoveContainerSetup';
import { MoveProgress } from './MoveProgress';
import type { ContainerTarget } from './move-dialog-types';
type MoveContainerDialogProps = {
  container: ContainerTarget | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};
export function MoveContainerDialog({ container, open: isOpen, onOpenChange, onSuccess }: MoveContainerDialogProps) {
  const { t } = useTranslation('containers');
  const currentEnvId = useEnvironmentStore((s) => s.currentId);
  const { data: environments = [] } = useEnvironmentList();
  const moveStream = useMoveContainerStream();
  const dockerEnvironments = useMemo(
    () => environments.filter((env) => env.id !== currentEnvId),
    [environments, currentEnvId],
  );
  const [targetEnvId, setTargetEnvId] = useState('');
  const [targetName, setTargetName] = useState('');
  const [transferImage, setTransferImage] = useState(true);
  const [createMissingNetworks, setCreateMissingNetworks] = useState(true);
  const [createMissingVolumes, setCreateMissingVolumes] = useState(true);
  const [copyNamedVolumes, setCopyNamedVolumes] = useState(true);
  const [stopSource, setStopSource] = useState(true);
  const [removeSource, setRemoveSource] = useState(false);
  const [startTarget, setStartTarget] = useState(true);
  const [networkMode, setNetworkMode] = useState('');
  const [networks, setNetworks] = useState<MoveNetworkConfig[]>([]);
  const [volumes, setVolumes] = useState<MoveVolumeConfig[]>([]);
  const [initializedFor, setInitializedFor] = useState<string | null>(null);
  const [seededNetworksFor, setSeededNetworksFor] = useState<unknown>(null);
  const [seededVolumesFor, setSeededVolumesFor] = useState<unknown>(null);
  const showProgress = moveStream.moving || moveStream.progress !== null;
  const defaultTargetEnvId = dockerEnvironments[0]?.id ?? '';
  if (!isOpen) {
    if (initializedFor !== null) setInitializedFor(null);
  } else if (!showProgress && container && initializedFor !== `${container.id}:${defaultTargetEnvId}`) {
    setInitializedFor(`${container.id}:${defaultTargetEnvId}`);
    setSeededNetworksFor(null);
    setSeededVolumesFor(null);
    setTargetEnvId(defaultTargetEnvId);
    setTargetName(container?.name ?? '');
    setTransferImage(true);
    setCreateMissingNetworks(true);
    setCreateMissingVolumes(true);
    setCopyNamedVolumes(true);
    setStopSource(true);
    setRemoveSource(false);
    setStartTarget(true);
    setNetworkMode('');
    setNetworks([]);
    setVolumes([]);
    moveStream.reset();
  }
  const planQuery = useMoveContainerPlan(
    container?.id ?? '',
    targetEnvId,
    targetName,
    networkMode,
    networks,
    volumes,
    isOpen,
  );
  const targetNetworksQuery = useQuery({
    queryKey: ['networks', targetEnvId],
    queryFn: () =>
      api
        .get<NetworkInfo[]>('/networks', targetEnvId ? { env: targetEnvId } : {})
        .then((response) => response.data ?? []),
    enabled: isOpen && !!targetEnvId,
    refetchInterval: 30_000,
  });
  const plan = planQuery.data;
  const targetOptions = dockerEnvironments.map((env) => ({ value: env.id, label: env.name }));
  const moveOptions = [
    {
      id: 'transfer-image',
      title: t('moveDialog.transferImage'),
      description: t('moveDialog.transferImageDesc'),
      checked: transferImage,
      onCheckedChange: setTransferImage,
    },
    {
      id: 'create-networks',
      title: t('moveDialog.createNetworks'),
      description: t('moveDialog.createNetworksDesc'),
      checked: createMissingNetworks,
      onCheckedChange: setCreateMissingNetworks,
    },
    {
      id: 'create-volumes',
      title: t('moveDialog.createVolumes'),
      description: t('moveDialog.createVolumesDesc'),
      checked: createMissingVolumes,
      onCheckedChange: setCreateMissingVolumes,
    },
    {
      id: 'copy-volumes',
      title: t('moveDialog.copyVolumes'),
      description: t('moveDialog.copyVolumesDesc'),
      checked: copyNamedVolumes,
      onCheckedChange: setCopyNamedVolumes,
    },
    {
      id: 'stop-source',
      title: t('moveDialog.stopSource'),
      description: t('moveDialog.stopSourceDesc'),
      checked: stopSource,
      onCheckedChange: setStopSource,
    },
    {
      id: 'remove-source',
      title: t('moveDialog.removeSource'),
      description: t('moveDialog.removeSourceDesc'),
      checked: removeSource,
      onCheckedChange: setRemoveSource,
    },
    {
      id: 'start-target',
      title: t('moveDialog.startTarget'),
      description: t('moveDialog.startTargetDesc'),
      checked: startTarget,
      onCheckedChange: setStartTarget,
    },
  ];

  if (isOpen && plan && networks.length === 0 && seededNetworksFor !== plan) {
    setSeededNetworksFor(plan);
    setNetworkMode(plan.networkMode ?? 'bridge');
    setNetworks(moveNetworkConfigsFromPlan(plan));
  }

  if (isOpen && plan && volumes.length === 0 && seededVolumesFor !== plan) {
    setSeededVolumesFor(plan);
    setVolumes(moveVolumeConfigsFromPlan(plan));
  }

  if (!container) return null;
  const handleTargetEnvChange = (value: string) => {
    setTargetEnvId(value);
    setNetworkMode('');
    setNetworks([]);
    setVolumes([]);
    setSeededNetworksFor(null);
    setSeededVolumesFor(null);
  };
  const handleMove = () => {
    moveStream.startMove(
      {
        id: container.id,
        targetEnvId,
        targetName,
        networkMode,
        networks,
        volumes,
        transferImage,
        createMissingNetworks,
        createMissingVolumes,
        copyNamedVolumes,
        stopSource,
        removeSource,
        startTarget,
      },
      {
        onDone: () => {
          onSuccess?.();
        },
      },
    );
  };
  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && moveStream.moving) return;
    if (!nextOpen) moveStream.reset();
    onOpenChange(nextOpen);
  };
  const handleProgressClose = () => {
    moveStream.reset();
    onOpenChange(false);
  };
  const handleProgressCancel = () => {
    moveStream.abort();
  };
  return (
    <Dialog open={isOpen} onOpenChange={handleDialogOpenChange}>
      <DialogContent className="max-w-3xl">
        <MoveContainerDialogHeader containerName={container.name} />
        <DialogBody className="space-y-4">
          {showProgress ? (
            <MoveProgress
              progress={moveStream.progress}
              logs={moveStream.logs}
              onClose={handleProgressClose}
              onCancel={moveStream.moving ? handleProgressCancel : undefined}
            />
          ) : (
            <MoveContainerSetup
              t={t}
              targetEnvId={targetEnvId}
              targetName={targetName}
              targetOptions={targetOptions}
              plan={plan}
              planLoading={planQuery.isLoading}
              fallbackImage={container.image}
              networkMode={networkMode}
              networks={networks}
              volumes={volumes}
              targetNetworks={targetNetworksQuery.data ?? []}
              moveOptions={moveOptions}
              onTargetEnvChange={handleTargetEnvChange}
              onTargetNameChange={setTargetName}
              onNetworkModeChange={setNetworkMode}
              onNetworksChange={setNetworks}
              onVolumesChange={setVolumes}
            />
          )}
        </DialogBody>
        {!showProgress && (
          <MoveContainerDialogFooter
            disabled={!targetEnvId || targetOptions.length === 0}
            onCancel={() => onOpenChange(false)}
            onMove={handleMove}
            t={t}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
