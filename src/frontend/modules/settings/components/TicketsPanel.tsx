// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconExternalLink, IconRefresh, IconRotateClockwise } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { type LinkState, useResyncLink, useRetryLink, useTicketLinks } from '../hooks/useItsm';

const STATE_VARIANT: Record<LinkState, 'warning' | 'destructive' | 'secondary' | 'success' | 'info'> = {
  pending: 'info',
  open: 'warning',
  remote_closed: 'destructive',
  resolved: 'success',
  orphaned: 'secondary',
};

const STATES: LinkState[] = ['pending', 'open', 'remote_closed', 'resolved', 'orphaned'];

export function TicketsPanel() {
  const { t } = useTranslation('settings');
  const [state, setState] = useState<LinkState | ''>('');
  const links = useTicketLinks(state);
  const resync = useResyncLink();
  const retry = useRetryLink();
  const rows = links.data ?? [];

  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <p className="max-w-2xl text-sm text-muted-foreground">{t('alerts.tickets.description')}</p>
        <Select
          value={state}
          onChange={(v) => setState(v as LinkState | '')}
          ariaLabel={t('alerts.tickets.stateFilter')}
          options={[
            { value: '', label: t('alerts.tickets.allStates') },
            ...STATES.map((s) => ({ value: s, label: t(`alerts.tickets.localState.${s}`) })),
          ]}
        />
      </div>
      {links.isLoading ? (
        <Spinner />
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('alerts.tickets.empty')}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('alerts.tickets.columns.state')}</TableHead>
              <TableHead>{t('alerts.tickets.columns.ticket')}</TableHead>
              <TableHead>{t('alerts.tickets.columns.remote')}</TableHead>
              <TableHead>{t('alerts.tickets.columns.alert')}</TableHead>
              <TableHead>{t('alerts.tickets.columns.channel')}</TableHead>
              <TableHead>{t('alerts.tickets.columns.synced')}</TableHead>
              <TableHead className="text-right">{t('alerts.tickets.columns.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((l) => (
              <TableRow key={l.id}>
                <TableCell>
                  <span className="flex items-center gap-2">
                    <Badge variant={STATE_VARIANT[l.localState]}>
                      {t(`alerts.tickets.localState.${l.localState}`)}
                    </Badge>
                    {l.parked && <Badge variant="destructive">{t('alerts.tickets.parked')}</Badge>}
                    <StatusError error={l.lastError} title={t('alerts.tickets.errorTitle')} />
                  </span>
                </TableCell>
                <TableCell>
                  {l.externalUrl ? (
                    <a
                      className="inline-flex items-center gap-1 text-primary underline"
                      href={l.externalUrl}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {l.externalNumber || l.externalId}
                      <IconExternalLink className="size-3" aria-hidden="true" />
                    </a>
                  ) : (
                    <span className="font-mono text-xs">{l.externalNumber || l.externalId || '—'}</span>
                  )}
                </TableCell>
                <TableCell>
                  {l.remoteState ? t(`alerts.tickets.remoteState.${l.remoteState}`) : '—'}
                  {l.remoteAssignee && <span className="ml-2 text-xs text-muted-foreground">{l.remoteAssignee}</span>}
                </TableCell>
                <TableCell>{l.alertName || l.alertId}</TableCell>
                <TableCell>{l.channelName || l.channelType}</TableCell>
                <TableCell>{l.lastSyncedAt ? timeAgo(l.lastSyncedAt) : '—'}</TableCell>
                <TableCell className="text-right">
                  <Button
                    variant="ghost"
                    size="icon"
                    permission={PERM.ALERTS_MANAGE}
                    aria-label={t('alerts.tickets.resync')}
                    title={t('alerts.tickets.resync')}
                    disabled={resync.isPending || !l.externalId}
                    onClick={() => resync.mutate(l.id)}
                  >
                    <IconRefresh className="size-4" />
                  </Button>
                  {l.parked && (
                    <Button
                      variant="ghost"
                      size="icon"
                      permission={PERM.ALERTS_MANAGE}
                      aria-label={t('alerts.tickets.retry')}
                      title={t('alerts.tickets.retry')}
                      disabled={retry.isPending}
                      onClick={() => retry.mutate(l.id)}
                    >
                      <IconRotateClockwise className="size-4" />
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
