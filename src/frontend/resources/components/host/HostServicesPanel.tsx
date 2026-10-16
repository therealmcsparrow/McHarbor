// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconReload } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import {
  useHostServiceAction,
  useHostServices,
  type HostService,
  type HostServiceAction,
  type HostServiceAvailability,
} from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { HostDaemonReloadDialog } from './HostDaemonReloadDialog';
import { HostServiceActionDialog, type HostServiceActionTarget } from './HostServiceActionDialog';
import { HostServiceOverrideDialog } from './HostServiceOverrideDialog';
import { HostServiceRevertDialog } from './HostServiceRevertDialog';
import { useHostServicesColumns } from './HostServicesColumns';
import { useHostServicesFilters } from './useHostServicesFilters';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type HostServicesPanelProps = {
  host: HostTarget;
  tableId: string;
};

function UnavailableNotice({ availability }: { availability: HostServiceAvailability }) {
  const { t } = useTranslation('environments');
  const reason = availability.reason ?? 'not_native';
  return (
    <Alert
      variant={reason === 'agent_outdated' || reason === 'self_unreachable' ? 'warning' : 'info'}
      title={t(`hostServices.unavailable.${reason}.title`)}
      description={t(`hostServices.unavailable.${reason}.description`, {
        version: availability.agentVersion || '—',
        build: availability.agentBuild || '—',
        minVersion: availability.minVersion,
        minBuild: availability.minBuild,
        detail: availability.detail || '—',
      })}
    />
  );
}

export function HostServicesPanel({ host, tableId }: HostServicesPanelProps) {
  const { t } = useTranslation('environments');
  const { data, isPending, isFetching, error, refetch } = useHostServices(host);
  const direct = useHostServiceAction(host);
  const [confirmation, setConfirm] = useState<HostServiceActionTarget | null>(null);
  const [editUnit, setEditUnit] = useState<string | null>(null);
  const [revertUnit, setRevertUnit] = useState<string | null>(null);
  const [reloadOpen, setReloadOpen] = useState(false);

  const units = data?.units ?? [];
  const filters = useHostServicesFilters(units);
  const busyUnit = direct.isPending ? (direct.variables?.unit ?? null) : null;

  const onAction = useCallback(
    (unit: HostService, action: HostServiceAction) => {
      if (action === 'start' || action === 'enable') {
        direct.mutate({ unit: unit.name, action });
        return;
      }
      setConfirm({
        unit: unit.name,
        action,
        isAgent: unit.isAgent,
        restartsMcHarbor: unit.restartsMcHarbor,
      });
    },
    [direct],
  );
  const onEdit = useCallback((unit: HostService) => setEditUnit(unit.name), []);
  const columns = useHostServicesColumns({ host, busyUnit, onAction, onEdit });

  if (data && !data.availability.supported) {
    return <UnavailableNotice availability={data.availability} />;
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      {data?.truncated && <Alert variant="warning" description={t('hostServices.truncated')} />}
      <TablePage
        id={tableId}
        header={false}
        title={t('hostServices.title')}
        search={{
          ...filters.search,
          placeholder: t('hostServices.searchPlaceholder'),
          fields: (unit) => [unit.name, unit.description, unit.activeState, unit.subState, unit.unitFileState],
        }}
        savedFilters={filters.savedFilters}
        facets={filters.facets}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-host-services' },
          actions: (
            <Button
              size="sm"
              variant="outline"
              permission={hostPermission(host, 'servicesManage')}
              onClick={() => setReloadOpen(true)}
            >
              <IconReload className="mr-1.5 size-4" aria-hidden="true" />
              {t('hostServices.action.daemonReload')}
            </Button>
          ),
        }}
        columns={columns}
        data={filters.rows}
        loading={isPending}
        error={error ?? undefined}
        onRetry={() => void refetch()}
        empty={{ title: t('hostServices.empty') }}
        getRowId={(unit) => unit.name}
        getRowTestId={(unit) => `host-service-${unit.name}`}
      />

      <HostServiceActionDialog
        host={host}
        target={confirmation}
        onOpenChange={(isOpen) => !isOpen && setConfirm(null)}
      />
      <HostServiceOverrideDialog
        host={host}
        unit={editUnit}
        onOpenChange={(isOpen) => !isOpen && setEditUnit(null)}
        onRevert={(unit) => setRevertUnit(unit)}
      />
      <HostServiceRevertDialog
        host={host}
        unit={revertUnit}
        onOpenChange={(isOpen) => !isOpen && setRevertUnit(null)}
        onSuccess={() => setEditUnit(null)}
      />
      <HostDaemonReloadDialog host={host} open={reloadOpen} onOpenChange={setReloadOpen} />
    </div>
  );
}
