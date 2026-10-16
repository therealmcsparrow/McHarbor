// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { statusSoft } from '@resources/theme/status-palette';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useUninstallPlugin, useUninstallPreview } from '../hooks/usePlugins';
import { UninstallImpactList } from './UninstallImpactList';
import type { InstalledPlugin, PluginEntry } from '../types/plugins';

interface UninstallConfirmDialogProps {
  plugin: InstalledPlugin | null;

  catalogEntry: PluginEntry | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
}

export function UninstallConfirmDialog({
  plugin,
  catalogEntry,
  open: isOpen,
  onOpenChange,
  onSuccess,
}: UninstallConfirmDialogProps) {
  const { t } = useTranslation('plugins');
  const purgeId = useId();
  const [purgeData, setPurgeData] = useState(false);
  const uninstall = useUninstallPlugin();
  const loading = uninstall.isPending;

  const [wasOpen, setWasOpen] = useState(false);
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen);
    if (isOpen) setPurgeData(false);
  }

  const slug = plugin?.slug ?? null;
  const preview = useUninstallPreview(slug);

  const displayName = catalogEntry?.name ?? plugin?.slug ?? '';

  const referencedRoles = preview.data?.permissions.referenced_by_roles ?? 0;
  const roleNames = preview.data?.permissions.roles ?? [];
  const dialogVariant: 'destructive' | 'caution' = referencedRoles > 0 ? 'caution' : 'destructive';

  // Uninstall drops the plugin's `plugin_<slug>_*` tables outright. For most
  // plugins that is unremarkable — a report can be regenerated. For some it is
  // the only copy of something an operator typed in by hand, so when the plugin
  // actually owns tables the dialog says so in its own right rather than
  // leaving it to a number in the impact list. A plugin that stores nothing
  // gets no banner it has not earned.
  const ownsData = (preview.data?.drop_targets.data_tables ?? 0) > 0;

  const title = t('confirm.uninstall_title', {
    name: displayName,
  });
  const description = t('confirm.uninstall_warning');

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      confirmLabel={t('confirm.uninstall_button', {})}
      variant={dialogVariant}
      permission={[PERM.PLUGINS_MANAGE, PERM.PLUGINS_DELETE]}
      mutation={uninstall}
      variables={{ slug: slug ?? '', purgeData }}
      blocked={!slug}
      onSuccess={() => onSuccess?.()}
    >
      {slug && (
        <div className="mt-4 space-y-3 text-sm">
          {preview.isLoading && <div className="text-xs text-muted-foreground">{t('confirm.preview_loading', {})}</div>}

          {preview.isError && (
            <ErrorState
              variant="banner"
              error={preview.error}
              description={t('confirm.preview_error', {})}
              onRetry={() => preview.refetch()}
            />
          )}

          {ownsData && (
            <div className={`flex items-start gap-3 rounded-lg p-3 ${statusSoft('danger')}`}>
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
              <p className="text-sm">{t('confirm.data_loss_warning')}</p>
            </div>
          )}

          {referencedRoles > 0 && (
            <div className={`flex items-start gap-3 rounded-lg p-3 ${statusSoft('warning')}`}>
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
              <p className="text-sm">
                {t('confirm.roles_strip_warning', {
                  count: referencedRoles,
                  roles: roleNames.join(', '),
                })}
              </p>
            </div>
          )}

          {preview.data && <UninstallImpactList preview={preview.data} />}

          <div className="flex items-start gap-3 rounded-lg border border-border p-3">
            <Checkbox
              id={purgeId}
              checked={purgeData}
              onCheckedChange={(next) => setPurgeData(next === true)}
              disabled={loading}
            />
            <div className="space-y-1">
              <Label htmlFor={purgeId} className="cursor-pointer">
                {t('confirm.purge_label')}
              </Label>
              <p className="text-xs text-muted-foreground">{t('confirm.purge_help')}</p>
            </div>
          </div>
        </div>
      )}
    </MutationConfirmDialog>
  );
}
