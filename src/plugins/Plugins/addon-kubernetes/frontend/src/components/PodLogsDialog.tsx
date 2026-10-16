// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconRefresh } from '@tabler/icons-react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { useSelectedCluster } from '../cluster-store';
import { fetchPodLogs, type ResourceRef } from '../hooks/useKubernetesActions';

type PodLogsDialogProps = {
  target: ResourceRef | null;
  onOpenChange: (open: boolean) => void;
};

const TAIL_OPTIONS = ['100', '500', '1000', '5000'];

export function PodLogsDialog({ target, onOpenChange }: PodLogsDialogProps) {
  const { t } = useTranslation('kubernetes');
  const envId = useSelectedCluster();
  const [tail, setTail] = useState('500');
  const [previous, setPrevious] = useState(false);

  const logs = useQuery({
    queryKey: ['kubernetes', 'logs', envId, target?.namespace, target?.name, tail, previous],
    enabled: Boolean(target),
    queryFn: () =>
      fetchPodLogs(envId, {
        namespace: target?.namespace ?? '',
        name: target?.name ?? '',
        tail: Number(tail),
        previous,
      }),
    // Logs are a point-in-time read; refetching under the reader would move the
    // scroll position out from under them.
    refetchOnWindowFocus: false,
  });

  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('logs.title')}</DialogTitle>
          <DialogDescription>
            {t('logs.description', { name: target?.name ?? '', namespace: target?.namespace ?? '' })}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-end gap-4">
          <div>
            <Label className="mb-2">{t('logs.tail')}</Label>
            <Select
              variant="outline"
              value={tail}
              onChange={setTail}
              ariaLabel={t('logs.tail')}
              className="min-w-[120px]"
              options={TAIL_OPTIONS.map((value) => ({ value, label: value }))}
            />
          </div>
          <div className="flex items-center gap-2 pb-2">
            <Switch id="k8s-logs-previous" checked={previous} onCheckedChange={setPrevious} />
            <Label htmlFor="k8s-logs-previous" className="cursor-pointer">
              {t('logs.previous')}
            </Label>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="mb-1"
            onClick={() => void logs.refetch()}
            disabled={logs.isFetching}
          >
            <IconRefresh className="size-4" />
            {t('actions.refresh')}
          </Button>
        </div>

        <div className="max-h-[55vh] overflow-auto rounded-lg border border-border bg-muted/30 p-4">
          {logs.isLoading && <p className="text-sm text-muted-foreground">{t('logs.loading')}</p>}
          {logs.error && <p className="text-sm text-destructive">{(logs.error as Error).message}</p>}
          {!logs.isLoading && !logs.error && (
            <pre className="font-mono text-xs break-words whitespace-pre-wrap text-foreground">
              {logs.data?.trim() ? logs.data : t('logs.empty')}
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
