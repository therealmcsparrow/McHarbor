// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { toastError } from '@resources/utils/toast';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useSnapshotConfigs,
  useCreateSnapshotConfig,
  useUpdateSnapshotConfig,
  useDeleteSnapshotConfig,
  useProbeSnapshotConfig,
  useRecentSnapshots,
  type SnapshotConfig,
  type SnapshotConfigWrite,
} from '../hooks/useSnapshotConfigs';
import { SnapshotConfigDialog } from './SnapshotConfigDialog';
import { SnapshotDetectionCard } from './SnapshotDetectionCard';
import { SnapshotConfigsCard } from './SnapshotConfigsCard';
import { RecentSnapshotsCard } from './RecentSnapshotsCard';
import type { SnapshotDriverStatus } from '../hooks/useSnapshotConfigs';

export function SnapshotsTab() {
  const { t } = useTranslation('settings');
  const envId = useEnvironmentStore((s) => s.currentId);
  const [createOpen, setCreateOpen] = useState(false);
  const [presetDriver, setPresetDriver] = useState<string | undefined>(undefined);
  const [editConfig, setEditConfig] = useState<SnapshotConfig | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<SnapshotConfig | null>(null);
  const [probingId, setProbingId] = useState<string | null>(null);

  const configs = useSnapshotConfigs(envId);
  const recent = useRecentSnapshots(envId, 20);
  const create = useCreateSnapshotConfig(envId);
  const update = useUpdateSnapshotConfig(envId);
  const remove = useDeleteSnapshotConfig(envId);
  const probe = useProbeSnapshotConfig(envId);

  const noEnv = !envId;

  function handleProbe(cfg: SnapshotConfig) {
    setProbingId(cfg.id);
    probe.mutate(cfg.id, {
      onSuccess: (data) => {
        if (data.available) {
          toast.success(t('snapshots.configs.probeSuccess', { count: data.targets?.length ?? 0 }));
        } else {
          const message = t('snapshots.configs.probeFailed', { error: data.error ?? 'unknown' });
          toastError(data.error ?? message, message);
        }
      },
      onSettled: () => setProbingId(null),
    });
  }

  function handleSave(body: SnapshotConfigWrite) {
    if (editConfig) {
      update.mutate(
        { id: editConfig.id, body },
        {
          onSuccess: () => {
            toast.success(t('snapshots.toast.configUpdated'));
            setEditConfig(null);
          },
        },
      );
    } else {
      create.mutate(body, {
        onSuccess: () => {
          toast.success(t('snapshots.toast.configCreated'));
          setCreateOpen(false);
        },
      });
    }
  }

  function handleConfirmDelete() {
    if (!deleteTarget) return;
    remove.mutate(deleteTarget.id, {
      onSuccess: () => {
        toast.success(t('snapshots.toast.configDeleted'));
        setDeleteTarget(null);
      },
    });
  }

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">{t('snapshots.description')}</p>

      <SnapshotDetectionCard
        envId={envId}
        onSetUp={(driver: SnapshotDriverStatus) => {
          setPresetDriver(driver.driver);
          setCreateOpen(true);
        }}
      />

      <SnapshotConfigsCard
        noEnv={noEnv}
        configs={configs}
        probingId={probingId}
        onAdd={() => {
          setPresetDriver(undefined);
          setCreateOpen(true);
        }}
        onProbe={handleProbe}
        onEdit={setEditConfig}
        onDelete={setDeleteTarget}
      />

      <RecentSnapshotsCard noEnv={noEnv} recent={recent} />

      {createOpen && (
        <SnapshotConfigDialog
          open
          presetDriver={presetDriver}
          onOpenChange={(isOpen) => !isOpen && setCreateOpen(false)}
          onSave={handleSave}
          saving={create.isPending}
        />
      )}
      {editConfig && (
        <SnapshotConfigDialog
          open
          config={editConfig}
          onOpenChange={(isOpen) => !isOpen && setEditConfig(null)}
          onSave={handleSave}
          saving={update.isPending}
        />
      )}
      {deleteTarget && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => !isOpen && setDeleteTarget(null)}
          title={t('snapshots.configs.confirmDeleteTitle')}
          description={t('snapshots.configs.confirmDeleteDescription', { name: deleteTarget.name })}
          confirmLabel={t('snapshots.configs.confirmDeleteLabel')}
          onConfirm={handleConfirmDelete}
          variant="destructive"
        />
      )}
    </div>
  );
}
