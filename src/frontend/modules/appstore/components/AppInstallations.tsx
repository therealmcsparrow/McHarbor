// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronRight, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { InstallProgress } from './InstallProgress';
import { useStreamUninstall } from '../hooks/useStreamUninstall';
import type { AppInstallation } from '../types';

interface AppInstallationsSummaryProps {
  installations: AppInstallation[];
  appName: string;
}

interface AppInstallationsListProps {
  installations: AppInstallation[];
}

function getEnvironmentLabel(installation: AppInstallation, fallback: string) {
  return installation.environmentName || installation.environmentId || fallback;
}

export function AppInstallationsSummary({ installations, appName }: AppInstallationsSummaryProps) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);

  if (installations.length === 0) {
    return null;
  }

  return (
    <>
      <div className="rounded-md border border-teal-500/30 bg-teal-500/10 px-3 py-2 text-teal-800 dark:text-teal-300">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className="text-[11px] font-semibold uppercase tracking-[0.12em]">
              {t('appStore.installedIn')}
            </span>
            <Badge variant="success" className="shrink-0">
              {installations.length}
            </Badge>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="size-7 shrink-0 text-teal-800 hover:bg-teal-500/15 dark:text-teal-300"
            aria-label={t('appStore.viewInstallations')}
            onClick={() => setOpen(true)}
          >
            <IconChevronRight className="size-4" />
          </Button>
        </div>
      </div>
      <AppInstallationsDialog
        appName={appName}
        installations={installations}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  );
}

function AppInstallationsDialog({
  appName,
  installations,
  open,
  onOpenChange,
}: {
  appName: string;
  installations: AppInstallation[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('common');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl p-0">
        <DialogHeader>
          <DialogTitle>{t('appStore.installationsTitle', { name: appName })}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-2 p-4">
          <AppInstallationsList installations={installations} />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export function AppInstallationsList({ installations }: AppInstallationsListProps) {
  const { t } = useTranslation('common');
  const { uninstalling, progress, logs, startUninstall, reset } = useStreamUninstall();
  const [selected, setSelected] = useState<AppInstallation | null>(null);
  const [removing, setRemoving] = useState<AppInstallation | null>(null);

  if (installations.length === 0) {
    return null;
  }

  const selectedEnvironment = selected
    ? getEnvironmentLabel(selected, t('appStore.unknownEnvironment'))
    : '';

  return (
    <>
      <div className="space-y-2">
        {installations.map((installation) => (
          <div
            key={installation.id || installation.stackId}
            className="rounded-md border border-border/60 bg-muted/20 px-3 py-2"
          >
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium text-foreground">
                  {installation.stackName || installation.stackId}
                </div>
                <div className="mt-0.5 truncate text-xs text-muted-foreground">
                  {t('appStore.environmentLabel')}: {getEnvironmentLabel(installation, t('appStore.unknownEnvironment'))}
                </div>
              </div>
              <Button
                size="sm"
                variant="destructive"
                className="h-8 shrink-0 gap-1"
                disabled={uninstalling}
                onClick={() => setSelected(installation)}
              >
                <IconTrash className="size-3.5" />
                {t('appStore.uninstall')}
              </Button>
            </div>
          </div>
        ))}
      </div>
      <ConfirmDialog
        open={!!selected}
        onOpenChange={(isOpen) => !isOpen && setSelected(null)}
        title={t('appStore.uninstallTitle')}
        description={t('appStore.uninstallDescription', {
          name: selected?.stackName ?? selected?.stackId ?? '',
          environment: selectedEnvironment,
        })}
        confirmLabel={t('appStore.uninstall')}
        loading={uninstalling}
        onConfirm={() => {
          if (!selected) return;
          setRemoving(selected);
          startUninstall(selected.id);
          setSelected(null);
        }}
      />
      {removing && (
        <div className="rounded-md border border-border bg-muted/20 px-3 py-2">
          <h4 className="text-sm font-medium text-foreground">
            {t('appStore.removalProgressTitle', {
              name: removing.stackName || removing.stackId,
            })}
          </h4>
          <InstallProgress
            progress={progress}
            logs={logs}
            onClose={() => {
              reset();
              setRemoving(null);
            }}
          />
        </div>
      )}
    </>
  );
}
