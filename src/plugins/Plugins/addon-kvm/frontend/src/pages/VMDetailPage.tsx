// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useDiagnose, useVM, useVMLifecycle, useVMStats } from '../hooks/useVMs';
import { VMBreadcrumb } from '../components/VMBreadcrumb';
import { VMDetailHeader } from '../components/VMDetailHeader';
import { VMDetailTabs, type DetailTabId } from '../components/VMDetailTabs';
import { VMLogsTab } from '../components/VMLogsTab';
import { VMOverviewTab } from '../components/VMOverviewTab';
import { VMConsole } from '../components/VMConsole';
import { SnapshotPanel } from '../components/SnapshotPanel';
import { SnapshotSchedules } from '../components/SnapshotSchedules';
import { HardwarePanel } from '../components/HardwarePanel';
import { BackupsPanel, type BackupIntent } from '../components/BackupsPanel';
import { SettingsTab } from '../components/SettingsTab';
import { DeleteVMDialog } from '../components/DeleteVMDialog';
import { CloneVMDialog } from '../components/CloneVMDialog';
import { MigrateVMDialog } from '../components/MigrateVMDialog';
import { DiagnosePanel } from '../components/DiagnosePanel';

interface VMDetailPageProps {
  id: string;
  onBack: () => void;
  initialTab?: DetailTabId;
  backupIntent?: BackupIntent;
  onIntentHandled?: () => void;
}

export function VMDetailPage({ id, onBack, initialTab, backupIntent, onIntentHandled }: VMDetailPageProps) {
  const { t } = useTranslation('vm');
  const { data: vm, isLoading, isError, refetch } = useVM(id);
  const lifecycle = useVMLifecycle();
  const diagnose = useDiagnose();
  const [deleting, setDeleting] = useState(false);
  const [dialog, setDialog] = useState<'clone' | 'migrate' | null>(null);
  const [activeTab, setActiveTab] = useState<DetailTabId>(initialTab ?? 'overview');

  const [seenTab, setSeenTab] = useState({ id, initialTab });
  if (seenTab.id !== id || seenTab.initialTab !== initialTab) {
    setSeenTab({ id, initialTab });
    if (initialTab) setActiveTab(initialTab);
  }

  const running = vm?.state === 'running';
  const statsQuery = useVMStats(id, Boolean(running));

  if (isLoading) return <Skeleton className="h-96 w-full" />;
  if (isError || !vm) return <ErrorState onRetry={() => void refetch()} />;

  return (
    <div className="flex h-full min-h-0 w-full flex-col gap-4">
      <VMBreadcrumb rootLabel={t('nav.machines')} leaf={vm.name} onRoot={onBack} />

      <VMDetailHeader
        vm={vm}
        busy={lifecycle.isPending}
        diagnosePending={diagnose.isPending}
        onAction={(action) => lifecycle.mutate({ id: vm.id, action })}
        onDiagnose={() => diagnose.mutate(vm.id)}
        onDelete={() => setDeleting(true)}
        onClone={() => setDialog('clone')}
        onMigrate={() => setDialog('migrate')}
      />

      <Card className="flex-none overflow-visible">
        <VMDetailTabs activeTab={activeTab} onTabChange={setActiveTab} />
      </Card>

      {!vm.hostAvailable && (
        <p className="rounded-md bg-warning/10 p-3 text-sm text-foreground">{t('state.unknownHint')}</p>
      )}

      {(diagnose.isPending || diagnose.data) && (
        <DiagnosePanel pending={diagnose.isPending} result={diagnose.data} onDismiss={() => diagnose.reset()} />
      )}

      <div
        id="vm-detail-panel"
        role="tabpanel"
        aria-labelledby={`vm-tab-${activeTab}`}
        className="flex min-h-0 flex-1 flex-col"
      >
        {activeTab === 'overview' && (
          <VMOverviewTab vm={vm} stats={statsQuery.data} statsUnavailable={statsQuery.isError} />
        )}
        {activeTab === 'console' && <VMConsole vm={vm} />}
        {activeTab === 'logs' && <VMLogsTab vm={vm} />}
        {activeTab === 'hardware' && <HardwarePanel vm={vm} />}
        {activeTab === 'snapshots' && (
          <div className="space-y-8">
            <SnapshotPanel vm={vm} />
            <SnapshotSchedules vm={vm} />
          </div>
        )}
        {activeTab === 'backups' && <BackupsPanel vm={vm} intent={backupIntent} onIntentHandled={onIntentHandled} />}
        {activeTab === 'settings' && <SettingsTab vm={vm} />}
      </div>

      {deleting && <DeleteVMDialog vm={vm} onClose={() => setDeleting(false)} onDeleted={onBack} />}
      {dialog === 'clone' && <CloneVMDialog vm={vm} onClose={() => setDialog(null)} />}
      {dialog === 'migrate' && <MigrateVMDialog vm={vm} onClose={() => setDialog(null)} />}
    </div>
  );
}
