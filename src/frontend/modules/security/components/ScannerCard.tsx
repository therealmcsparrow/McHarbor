// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useIsNative } from '@resources/hooks/useInstallKind';
import { IconPencil, IconPlayerPlay, IconPuzzle, IconTrash } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteScanner, useTestScanner, type ScannerConfig } from '../hooks/useScanners';
import { ScannerTestDialog } from './ScannerTestDialog';

type ScannerCardProps = {
  scanner: ScannerConfig;
  onEdit: (scanner: ScannerConfig) => void;
};

/**
 * Pick the one field worth showing on a collapsed card.
 *
 * Every engine describes itself differently — Clair has a server URL,
 * Trivy has a severity floor — so this takes the first of a few
 * conventional keys rather than hard-coding one engine's shape. A
 * future adapter declaring `server_url` or `severity` gets a summary
 * line for free, and one declaring neither simply shows none.
 */
function scannerSummary(scanner: ScannerConfig): string {
  for (const key of ['server_url', 'url', 'endpoint_url', 'severity', 'scope']) {
    const field = scanner.fields.find((f) => f.key === key);
    if (field?.value) return field.value;
  }
  return '';
}

/**
 * One configured scanner, collapsed to a summary row.
 *
 * The title is the operator's own name for the engine, with the adapter
 * underneath — several rows may share an engine, so "Clair (production)"
 * is what distinguishes them, not "Clair".
 *
 * Three states get their own badge because they have three different
 * fixes, and collapsing them into one "unavailable" is what sends an
 * operator looking in the wrong place: the plugin is gone (reinstall
 * it), the binary is missing (rebuild the image), the engine is off
 * (switch it on).
 */
export function ScannerCard({ scanner, onEdit }: ScannerCardProps) {
  const { t } = useTranslation('security');
  const native = useIsNative();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [testOpen, setTestOpen] = useState(false);

  const test = useTestScanner();
  const remove = useDeleteScanner();

  const summary = scannerSummary(scanner);

  const runDelete = () => {
    remove.mutate(scanner.id, {
      onSuccess: () => {
        setDeleteOpen(false);
        toast.success(t('scanning.deleted'));
      },
    });
  };

  // The modal opens first and shows a spinner, so the operator sees the
  // test start rather than waiting on a button that looks inert for as
  // long as it takes a Clair server to answer.
  //
  // Only a request that could not run at all raises a toast now (the global
  // MutationCache raises it; onError only closes the modal). A
  // *failing* test is a successful response carrying a step report, and
  // the modal is where that gets read.
  const closeTest = () => setTestOpen(false);

  const runTest = () => {
    setTestOpen(true);
    test.mutate(scanner.id, { onError: closeTest });
  };

  return (
    <div className="mc-glass rounded-lg p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-primary">
            {scanner.slug && scanner.icon ? (
              <PluginAssetImage slug={scanner.slug} icon={scanner.icon} className="size-5" />
            ) : (
              <IconPuzzle className="size-5" />
            )}
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-medium text-foreground">{scanner.name}</h3>
              {scanner.isDefault && (
                <Badge variant="default" className="py-1">
                  {t('scanning.default')}
                </Badge>
              )}
              <Badge variant={scanner.enabled ? 'success' : 'secondary'} className="py-1">
                {scanner.enabled ? t('scanning.enabled') : t('scanning.disabled')}
              </Badge>
              {!scanner.installed ? (
                <Badge variant="destructive" className="py-1">
                  {t('scanning.pluginMissing')}
                </Badge>
              ) : (
                !scanner.available && (
                  <Badge variant="destructive" className="py-1">
                    {scanner.requiresLocalBinary ? t('scanning.binaryMissing') : t('scanning.notConfigured')}
                  </Badge>
                )
              )}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{scanner.displayName}</p>
            {native && scanner.installed && !scanner.available && scanner.requiresLocalBinary && (
              <p className="mt-1 text-xs text-muted-foreground">
                {t('scanning.binaryMissingNativeHint', { engine: scanner.displayName })}
              </p>
            )}
            {summary && <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{summary}</p>}
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap gap-2">
          {scanner.supportsTest && (
            <Button
              permission={PERM.SETTINGS_MANAGE}
              variant="outline"
              size="sm"
              onClick={runTest}
              disabled={test.isPending || !scanner.installed}
            >
              {test.isPending ? <Spinner size="sm" /> : <IconPlayerPlay className="size-4" />}
              {t('scanning.test.button')}
            </Button>
          )}
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            size="sm"
            onClick={() => onEdit(scanner)}
            disabled={!scanner.installed}
          >
            <IconPencil className="size-4" />
            {t('scanning.edit')}
          </Button>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('scanning.deleteScanner')}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ScannerTestDialog
        open={testOpen}
        onOpenChange={(isOpen) => {
          setTestOpen(isOpen);
          // Drop the previous report on close so reopening the modal
          // never shows a stale result from an earlier run while the
          // new one is still in flight.
          if (!isOpen) test.reset();
        }}
        result={test.data}
        pending={test.isPending}
        scannerName={scanner.name}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('scanning.confirmDeleteTitle')}
        description={t('scanning.confirmDeleteDescription', { name: scanner.name })}
        confirmLabel={t('scanning.confirmDeleteLabel')}
        loading={remove.isPending}
        onConfirm={runDelete}
      />
    </div>
  );
}
