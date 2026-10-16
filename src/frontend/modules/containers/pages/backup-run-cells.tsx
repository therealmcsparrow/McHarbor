// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Cell renderers for the backup runs table. Extracted from
// backup-columns.tsx unchanged, including the per-button `data-testid`
// hooks the e2e specs select on. The destination strip and the action
// buttons live in backup-run-row-cells.tsx.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconExclamationCircle, IconExternalLink, IconInfoCircle } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import type { ContainerBackupRun, ContainerBackupRunDestination } from '@resources/hooks/useContainerBackups';
import { destinationLabel, runHasError, runTypeKey, statusBadgeVariant } from './backups-page-helpers';
import { StaleIdBadge } from '../components/StaleIdBadge';
import { UploadedBadge } from '../components/UploadedBadge';
import { isContainerRun, type BackupRunRow } from './backup-run-rows';

export const DASH = '—';

// destinationLabelFor binds the shared fallback so the destination cell
// does not have to repeat the placeholder on every call site.
export function destinationLabelFor(destination: ContainerBackupRunDestination): string {
  return destinationLabel(destination, DASH);
}

export function RunContainerCell({
  run,
  onOpenContainer,
}: {
  run: ContainerBackupRun;
  onOpenContainer: (environmentId: string, containerId: string) => void;
}) {
  const shortId = run.containerId.slice(0, 12);
  return (
    <Link
      to={`/containers/${run.containerId}`}
      onClick={(clickEvent) => {
        // Switch to the run's owning environment before navigating so
        // the detail page queries the right orchestrator.
        clickEvent.preventDefault();
        clickEvent.stopPropagation();
        onOpenContainer(run.environmentId, run.containerId);
      }}
      className="flex flex-col items-start gap-0.5 font-medium text-foreground hover:text-primary"
    >
      <span className="flex items-center gap-1">
        {run.containerName || shortId}
        <IconExternalLink className="size-3.5 opacity-60" aria-hidden="true" />
      </span>
      {/* The id only earns a subtitle line when the title is a name —
          otherwise it would print the same short id twice. */}
      {run.containerName && <span className="font-mono text-xs font-normal text-muted-foreground">{shortId}</span>}
      {run.containerIdStale && <StaleIdBadge />}
    </Link>
  );
}

export function RunStatusCell({
  run,
  onShowError,
}: {
  run: BackupRunRow;
  onShowError: (run: ContainerBackupRun) => void;
}) {
  const { t } = useTranslation('containers');
  const hasError = runHasError(run);
  return (
    <div className="flex items-center gap-1">
      <Badge variant={statusBadgeVariant(run.status)}>
        {run.status === 'running' && <Spinner size="sm" className="mr-1.5" />}
        {t(`backups.status.${run.status}`)}
      </Badge>
      {!!run.warning?.trim() && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="inline-flex size-7 cursor-default items-center justify-center text-warning"
              aria-label={run.warning}
            >
              <IconExclamationCircle className="size-4" aria-hidden="true" />
            </span>
          </TooltipTrigger>
          <TooltipContent>{run.warning}</TooltipContent>
        </Tooltip>
      )}
      {hasError && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('backups.errorDialog.open')}
          onClick={() => onShowError(run)}
          data-testid={`run-error-info-${run.id}`}
        >
          <IconAlertTriangle className="size-4 text-destructive" aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}

export function RunTypeCell({
  run,
  onShowItems,
}: {
  run: BackupRunRow;
  onShowItems: (run: ContainerBackupRun) => void;
}) {
  const { t } = useTranslation('containers');
  // Only a partial run gets the info button: a full run's contents
  // are already stated by its label, and an unlabelled historical
  // run has nothing recorded to show.
  const showItems = isContainerRun(run) && run.scope === 'partial';
  return (
    <div className="flex items-center gap-1">
      <Badge variant={run.operation === 'restore' ? 'outline' : 'secondary'}>{t(runTypeKey(run))}</Badge>
      {run.origin === 'upload' && <UploadedBadge runId={run.id} />}
      {!!run.providerKind && (
        <Badge variant="outline" className="font-mono text-[10px]">
          {run.providerKind}
        </Badge>
      )}
      {showItems && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('backups.itemsDialog.open')}
          onClick={() => onShowItems(run)}
          data-testid={`run-items-info-${run.id}`}
        >
          <IconInfoCircle className="size-4" aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
