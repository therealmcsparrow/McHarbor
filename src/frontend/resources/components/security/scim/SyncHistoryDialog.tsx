// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconHistory } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatDate } from '@resources/utils/format';
import { useEndpointHistory, type Endpoint, type SyncLogEntry } from '@resources/hooks/useScimEndpoints';

const STATUS_VARIANT: Record<string, 'success' | 'destructive' | 'neutral'> = {
  success: 'success',
  failed: 'destructive',
  running: 'neutral',
};

const TRIGGERS = ['schedule', 'manual', 'preview'];

function HistoryRow({ entry }: { entry: SyncLogEntry }) {
  const { t } = useTranslation('scim');
  const trigger = TRIGGERS.includes(entry.trigger) ? entry.trigger : 'schedule';
  return (
    <li className="space-y-1 rounded-md border border-border p-3 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={STATUS_VARIANT[entry.status] ?? 'neutral'}>
          {t(`syncStatus.${STATUS_VARIANT[entry.status] ? entry.status : 'pending'}`)}
        </Badge>
        <StatusError error={entry.error} title={t('history.errorTitle')} />
        {entry.dryRun && <Badge variant="info">{t('history.dryRun')}</Badge>}
        <Badge variant="outline">{t(`history.trigger.${trigger}`)}</Badge>
        <span className="ml-auto text-muted-foreground">{formatDate(entry.startedAt)}</span>
      </div>
      <p className="text-muted-foreground">
        {t('history.counts', {
          seen: entry.usersSeen,
          linked: entry.usersLinked,
          unmatched: entry.usersUnmatched,
          updated: entry.usersUpdated,
          disabled: entry.usersDisabled,
          skipped: entry.usersSkipped,
          groups: entry.groupsSeen,
        })}
      </p>
      {entry.notes.length > 0 && (
        <ul className="list-inside list-disc text-muted-foreground" aria-label={t('history.notes')}>
          {Array.from(new Set(entry.notes)).map((note) => (
            <li key={note} className="break-words">
              {note}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

export function SyncHistoryDialog({ endpoint, onClose }: { endpoint: Endpoint | null; onClose: () => void }) {
  const { t } = useTranslation('scim');
  const { data, isLoading } = useEndpointHistory(endpoint?.id ?? '');

  return (
    <Dialog open={!!endpoint} onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <IconHistory className="mr-1 inline size-4" />
            {t('history.title', { name: endpoint?.name ?? '' })}
          </DialogTitle>
          <DialogDescription>{t('history.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[60vh] overflow-y-auto">
          {isLoading ? (
            <div className="flex justify-center py-8">
              <Spinner />
            </div>
          ) : !data || data.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">{t('history.empty')}</p>
          ) : (
            <ul className="space-y-3">
              {data.map((entry) => (
                <HistoryRow key={entry.id} entry={entry} />
              ))}
            </ul>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('history.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
