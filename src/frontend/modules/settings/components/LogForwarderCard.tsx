// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPencil, IconPuzzle, IconSend, IconTrash } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteLogForwarder, useTestLogForwarder, type LogForwarder } from '../hooks/useLogForwarders';
import { LogForwarderHistory } from './LogForwarderHistory';

type LogForwarderCardProps = {
  forwarder: LogForwarder;
  onEdit: (forwarder: LogForwarder) => void;
};

/**
 * Pick the one field worth showing on a collapsed card: where events
 * actually go. Every adapter names its destination differently, so this
 * takes the first of the conventional keys rather than hard-coding one
 * transport's shape — a new forwarder declaring `endpoint_url` or `host`
 * gets a summary line for free, and one declaring neither simply shows
 * none.
 */
function destinationSummary(forwarder: LogForwarder): string {
  for (const key of ['endpoint_url', 'host', 'url', 'server_url']) {
    const field = forwarder.fields.find((f) => f.key === key);
    if (field?.value) return field.value;
  }
  return '';
}

/**
 * One configured forwarder, collapsed to a summary row.
 *
 * The title is the operator's own name for the destination, with the
 * adapter underneath — several rows may share a type, so "Splunk
 * (production)" is what distinguishes them, not "SIEM".
 *
 * Configuration lives in the dialog, matching how storage locations and
 * communication channels work: the tab answers "what is set up and is it
 * working", the dialog answers "change it". Delivery history stays here
 * but behind a toggle — it is the answer to "is this sink actually
 * receiving events", which you want occasionally, not on every render.
 */
export function LogForwarderCard({ forwarder, onEdit }: LogForwarderCardProps) {
  const { t } = useTranslation('settings');
  const [deleteOpen, setDeleteOpen] = useState(false);

  const test = useTestLogForwarder();
  const remove = useDeleteLogForwarder();

  const summary = destinationSummary(forwarder);

  const runDelete = () => {
    remove.mutate(forwarder.id, {
      onSuccess: () => {
        setDeleteOpen(false);
        toast.success(t('logging.deleted'));
      },
    });
  };

  const runTest = () => {
    test.mutate(forwarder.id, {
      onSuccess: () => toast.success(t('logging.testSuccess')),
    });
  };

  return (
    <div className="mc-glass rounded-lg p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-primary">
            {forwarder.slug && forwarder.icon ? (
              <PluginAssetImage slug={forwarder.slug} icon={forwarder.icon} className="size-5" />
            ) : (
              <IconPuzzle className="size-5" />
            )}
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-medium text-foreground">{forwarder.name}</h3>
              <Badge variant={forwarder.enabled ? 'success' : 'secondary'} className="py-1">
                {forwarder.enabled ? t('logging.enabled') : t('logging.disabled')}
              </Badge>
              {!forwarder.available && (
                <Badge variant="destructive" className="py-1">
                  {t('logging.adapterUnavailable')}
                </Badge>
              )}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{forwarder.displayName}</p>
            {summary && <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{summary}</p>}
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap gap-2">
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            size="sm"
            onClick={runTest}
            disabled={test.isPending || !forwarder.enabled || !forwarder.available}
          >
            {test.isPending ? <Spinner size="sm" /> : <IconSend className="size-4" />}
            {t('logging.test')}
          </Button>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            size="sm"
            onClick={() => onEdit(forwarder)}
            disabled={!forwarder.available}
          >
            <IconPencil className="size-4" />
            {t('logging.edit')}
          </Button>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('logging.deleteForwarder')}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('logging.confirmDeleteTitle')}
        description={t('logging.confirmDeleteDescription', { name: forwarder.name })}
        confirmLabel={t('logging.confirmDeleteLabel')}
        loading={remove.isPending}
        onConfirm={runDelete}
      />

      <LogForwarderHistory
        forwarderId={forwarder.id}
        supported={forwarder.available && forwarder.supportsDeliveryHistory}
      />
    </div>
  );
}
