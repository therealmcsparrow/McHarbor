// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandGit, IconListCheck, IconPlus, IconReportAnalytics, IconSettings } from '@tabler/icons-react';
import { Link } from 'react-router';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  type DesiredState,
  useDesiredStates,
  useReconcile,
  useCheckDrift,
  useCreateDesiredState,
  useDeleteDesiredState,
} from '../hooks/useReconciler';
import { CreateDesiredStateDialog } from '../components/CreateDesiredStateDialog';
import { useReconcilerColumns } from '../components/useReconcilerColumns';
import { DriftDetailsDialog } from '../components/DriftDetailsDialog';
import { PERM } from '@core/auth/permissionKeys';
import { ModeDialog } from '../components/ModeDialog';
import { ProposalDialog } from '../components/ProposalDialog';
import { PullRequestDialog } from '../components/PullRequestDialog';
import { ReconcilerSettingsDialog } from '../components/ReconcilerSettingsDialog';
import { type Proposal, usePropose } from '../hooks/useProposals';

export default function ReconcilerPage() {
  const { t } = useTranslation('common');
  const { t: tr } = useTranslation('reconciler');
  const { data: states = [], isLoading, isFetching, refetch } = useDesiredStates();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const reconcile = useReconcile(t);
  const checkDrift = useCheckDrift(t);
  const createState = useCreateDesiredState(t);
  const deleteState = useDeleteDesiredState(t);

  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);

  const [driftOf, setDriftOf] = useState<DesiredState | null>(null);
  const onDelete = useCallback((id: string) => setConfirmTarget(id), []);
  const onShowDrift = useCallback((state: DesiredState) => setDriftOf(state), []);
  const [modeOf, setModeOf] = useState<DesiredState | null>(null);
  const [prOf, setPrOf] = useState<DesiredState | null>(null);
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const propose = usePropose();
  const onMode = useCallback((state: DesiredState) => setModeOf(state), []);
  const onPullRequest = useCallback((state: DesiredState) => setPrOf(state), []);
  const onPropose = useCallback(
    (state: DesiredState) => propose.mutate(state.id, { onSuccess: setProposal }),
    [propose],
  );
  const columns = useReconcilerColumns({
    reconcile,
    checkDrift,
    onDelete,
    onShowDrift,
    onMode,
    onPropose,
    onPullRequest,
  });

  return (
    <>
      <TablePage
        id="reconciler"
        title={t('reconciler.title')}
        description={t('reconciler.description')}
        actions={
          <div className="flex items-center gap-2">
            <Button asChild variant="outline">
              <Link to="/reconciler/report">
                <IconReportAnalytics className="h-4 w-4" /> {tr('report.open')}
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link to="/reconciler/proposals">
                <IconListCheck className="h-4 w-4" /> {tr('proposals.open')}
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link to="/reconciler/sources">
                <IconBrandGit className="h-4 w-4" /> {tr('sources.open')}
              </Link>
            </Button>
            <Button
              variant="outline"
              size="icon"
              aria-label={tr('settings.title')}
              title={tr('settings.title')}
              onClick={() => setSettingsOpen(true)}
            >
              <IconSettings className="h-4 w-4" />
            </Button>
            <Button permission={PERM.RECONCILER_MANAGE} onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('reconciler.addDesiredState')}
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('reconciler.searchPlaceholder'),
          fields: (state) => [
            state.name,
            state.description,
            state.containerName,
            state.imageRef,
            state.desiredStatus,
            state.stackName,
          ],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-desired-states' },
        }}
        columns={columns}
        data={states}
        loading={isLoading}
        empty={{ title: t('reconciler.noDesiredStates') }}
        getRowId={(state) => state.id}
      />

      <CreateDesiredStateDialog open={createOpen} onOpenChange={setCreateOpen} createState={createState} />

      <DriftDetailsDialog state={driftOf} onOpenChange={(isOpen2) => !isOpen2 && setDriftOf(null)} />
      {modeOf && <ModeDialog state={modeOf} onOpenChange={(isOpen2) => !isOpen2 && setModeOf(null)} />}
      {prOf && <PullRequestDialog state={prOf} onOpenChange={(dialogOpen) => !dialogOpen && setPrOf(null)} />}
      {proposal && <ProposalDialog proposal={proposal} onOpenChange={(openState) => !openState && setProposal(null)} />}
      {settingsOpen && <ReconcilerSettingsDialog onOpenChange={setSettingsOpen} />}

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(isOpen) => !isOpen && setConfirmTarget(null)}
        title={t('reconciler.deleteTitle')}
        description={t('reconciler.deleteDescription')}
        confirmLabel={t('actions.delete')}
        onConfirm={() => {
          if (confirmTarget) deleteState.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={deleteState.isPending}
      />
    </>
  );
}
