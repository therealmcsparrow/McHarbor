// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { StatusError } from '@resources/components/ui/StatusError';
import { timeAgo } from '@resources/utils/format';
import { type Proposal, type ProposalStatus, useProposals } from '../hooks/useProposals';
import { ProposalDialog } from '../components/ProposalDialog';

const STATUSES: ProposalStatus[] = ['pending', 'applied', 'failed', 'rejected', 'superseded'];
const STATUS_VARIANT = {
  pending: 'warning',
  applying: 'secondary',
  applied: 'success',
  failed: 'destructive',
  rejected: 'secondary',
  superseded: 'secondary',
} as const;

export default function ProposalsPage() {
  const { t } = useTranslation('reconciler');
  const [statusValue, setStatus] = useState<ProposalStatus | ''>('pending');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [isOpen, setOpen] = useState<Proposal | null>(null);
  const proposals = useProposals(statusValue);

  const columns = useMemo<ColumnDef<Proposal, unknown>[]>(
    () => [
      {
        accessorKey: 'desiredName',
        header: t('proposals.columns.watch'),
        cell: ({ row }) => (
          <Button variant="link" className="h-auto p-0 font-medium" onClick={() => setOpen(row.original)}>
            {row.original.desiredName}
          </Button>
        ),
      },
      {
        accessorKey: 'status',
        header: t('proposals.columns.status'),
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <Badge variant={STATUS_VARIANT[row.original.status]}>{t(`proposals.status.${row.original.status}`)}</Badge>
            <StatusError error={row.original.error} title={t('proposals.errorTitle')} />
          </span>
        ),
      },
      {
        accessorKey: 'origin',
        header: t('proposals.columns.origin'),
        cell: ({ row }) => t(`proposals.origin.${row.original.origin}`),
      },
      {
        id: 'steps',
        header: t('proposals.columns.steps'),
        meta: { sortValue: (p: Proposal) => p.steps.map((s) => s.action).join(', ') },
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.steps.map((s) => t(`step.${s.action}`)).join(', ')}
          </span>
        ),
      },
      {
        accessorKey: 'createdAt',
        header: t('proposals.columns.created'),
        cell: ({ row }) => timeAgo(row.original.createdAt),
      },
      {
        accessorKey: 'resolvedBy',
        header: t('proposals.columns.resolvedBy'),
        cell: ({ row }) => row.original.resolvedBy || '—',
      },
    ],
    [t],
  );

  return (
    <>
      <TablePage
        id="reconciler-proposals"
        title={t('proposals.title')}
        description={t('proposals.description')}
        actions={
          <div className="flex items-center gap-2">
            <Select
              value={statusValue}
              onChange={(value) => setStatus(value as ProposalStatus | '')}
              ariaLabel={t('proposals.statusFilter')}
              options={[
                { value: '', label: t('proposals.allStatuses') },
                ...STATUSES.map((s) => ({ value: s, label: t(`proposals.status.${s}`) })),
              ]}
            />
            <Button asChild variant="outline">
              <Link to="/reconciler">
                <IconArrowLeft className="h-4 w-4" /> {t('report.back')}
              </Link>
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('proposals.columns.watch'),
          fields: (p) => [p.desiredName, p.status, p.resolvedBy ?? ''],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void proposals.refetch(), isFetching: proposals.isFetching },
          export: { filename: 'mcharbor-reconcile-proposals' },
        }}
        columns={columns}
        data={proposals.data ?? []}
        loading={proposals.isLoading}
        empty={{ title: t('proposals.empty') }}
        getRowId={(p) => p.id}
      />
      {isOpen && <ProposalDialog proposal={isOpen} onOpenChange={(o) => !o && setOpen(null)} />}
    </>
  );
}
