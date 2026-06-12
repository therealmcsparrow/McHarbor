// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { NetworkInfo } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import {
  Dialog,
  DialogBody,
  DialogContent,
} from '@resources/components/ui/Dialog';
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
export function MoveContainerDialog({ container, open, onOpenChange, onSuccess }: MoveContainerDialogProps) {
  const { t } = useTranslation('containers');
  const currentEnvId = useEnvironmentStore((s) => s.currentId);
  const { data: environments = [] } = useEnvironmentList();
  const moveStream = useMoveContainerStream();
  const dockerEnvironments = useMemo(
    () => environments.filter((env) => env.orchestratorType === 'docker' && env.id !== currentEnvId),
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
  const initializedForRef = useRef<string | null>(null);
  const showProgress = moveStream.moving || moveStream.progress !== null;
  const defaultTargetEnvId = dockerEnvironments[0]?.id ?? '';
  useEffect(() => {
    if (!open) {
      initializedForRef.current = null;
      return;
    }
    if (showProgress || !container) return;

    const initKey = `${container.id}:${defaultTargetEnvId}`;
    if (initializedForRef.current === initKey) return;
    initializedForRef.current = initKey;

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
  }, [open, showProgress, container, defaultTargetEnvId, moveStream]);
  const planQuery = useMoveContainerPlan(container?.id ?? '', targetEnvId, targetName, networkMode, networks, volumes, open);
  const targetNetworksQuery = useQuery({
    queryKey: ['networks', targetEnvId],
    queryFn: () =>
      api
        .get<NetworkInfo[]>('/networks', targetEnvId ? { env: targetEnvId } : {})
        .then((response) => response.data ?? []),
    enabled: open && !!targetEnvId,
    refetchInterval: 30_000,
  });
  const plan = planQuery.data;
  const targetOptions = dockerEnvironments.map((env) => ({ value: env.id, label: env.name }));
  const moveOptions = [
    { id: 'transfer-image', title: t('moveDialog.transferImage'), description: t('moveDialog.transferImageDesc'), checked: transferImage, onCheckedChange: setTransferImage },
    { id: 'create-networks', title: t('moveDialog.createNetworks'), description: t('moveDialog.createNetworksDesc'), checked: createMissingNetworks, onCheckedChange: setCreateMissingNetworks },
    { id: 'create-volumes', title: t('moveDialog.createVolumes'), description: t('moveDialog.createVolumesDesc'), checked: createMissingVolumes, onCheckedChange: setCreateMissingVolumes },
    { id: 'copy-volumes', title: t('moveDialog.copyVolumes'), description: t('moveDialog.copyVolumesDesc'), checked: copyNamedVolumes, onCheckedChange: setCopyNamedVolumes },
    { id: 'stop-source', title: t('moveDialog.stopSource'), description: t('moveDialog.stopSourceDesc'), checked: stopSource, onCheckedChange: setStopSource },
    { id: 'remove-source', title: t('moveDialog.removeSource'), description: t('moveDialog.removeSourceDesc'), checked: removeSource, onCheckedChange: setRemoveSource },
    { id: 'start-target', title: t('moveDialog.startTarget'), description: t('moveDialog.startTargetDesc'), checked: startTarget, onCheckedChange: setStartTarget },
  ];

  useEffect(() => {
    if (!open || !plan || networks.length > 0) return;
    setNetworkMode(plan.networkMode ?? 'bridge');
    setNetworks(moveNetworkConfigsFromPlan(plan));
  }, [open, plan, networks.length]);

  useEffect(() => {
    if (!open || !plan || volumes.length > 0) return;
    setVolumes(moveVolumeConfigsFromPlan(plan));
  }, [open, plan, volumes.length]);

  if (!container) return null;
  const handleTargetEnvChange = (value: string) => {
    setTargetEnvId(value);
    setNetworkMode('');
    setNetworks([]);
    setVolumes([]);
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
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
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
