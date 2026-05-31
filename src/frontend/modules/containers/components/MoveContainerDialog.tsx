// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation, Trans } from 'react-i18next';
import { IconArrowsExchange } from '@tabler/icons-react';
import { api } from '@core/api/client';
import type { NetworkInfo } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import type { MoveNetworkConfig } from '../hooks/useContainers';
import { useMoveContainerPlan, useMoveContainerStream } from '../hooks/useContainers';
import { moveNetworkConfigsFromPlan } from './MoveNetworkSettings';
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
    moveStream.reset();
  }, [open, showProgress, container, defaultTargetEnvId, moveStream.reset]);
  const planQuery = useMoveContainerPlan(container?.id ?? '', targetEnvId, targetName, networkMode, networks, open);
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

  if (!container) return null;
  const handleTargetEnvChange = (value: string) => {
    setTargetEnvId(value);
    setNetworkMode('');
    setNetworks([]);
  };
  const handleMove = () => {
    moveStream.startMove(
      {
        id: container.id,
        targetEnvId,
        targetName,
        networkMode,
        networks,
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
  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconArrowsExchange className="size-4 text-primary" />
            {t('moveDialog.title')}
          </DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="moveDialog.description"
              ns="containers"
              values={{ name: container.name }}
              components={{ bold: <span className="font-medium text-foreground" /> }}
            />
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {showProgress ? (
            <MoveProgress progress={moveStream.progress} logs={moveStream.logs} onClose={handleProgressClose} />
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
              targetNetworks={targetNetworksQuery.data ?? []}
              moveOptions={moveOptions}
              onTargetEnvChange={handleTargetEnvChange}
              onTargetNameChange={setTargetName}
              onNetworkModeChange={setNetworkMode}
              onNetworksChange={setNetworks}
            />
          )}
        </DialogBody>
        {!showProgress && (
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('actions.cancel', { ns: 'common' })}
            </Button>
            <Button onClick={handleMove} disabled={!targetEnvId || targetOptions.length === 0}>
              {t('moveDialog.move')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
