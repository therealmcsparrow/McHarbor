// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusBadge } from './StatusBadge';
import { useSwarmTaskLogs } from '../hooks/useSwarmTasks';
import { taskStateTone } from '../format';
import type { SwarmTask } from '../types';

type TaskLogsDialogProps = {
  task: SwarmTask | null;
  onOpenChange: (open: boolean) => void;
};

/** One task's output, plus the state and error that explain why it has any. */
export function TaskLogsDialog({ task, onOpenChange }: TaskLogsDialogProps) {
  const { t } = useTranslation('swarm');
  const logs = useSwarmTaskLogs(task?.id, 500, task !== null);

  return (
    <Dialog open={task !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{task?.name ?? ''}</DialogTitle>
          <DialogDescription>
            {t('dialogs.taskLogs.description', {
              service: task?.serviceName ?? '',
              node: task?.nodeName ?? '',
            })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {task && (
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge tone={taskStateTone(task.state)}>{t(`taskState.${task.state}`)}</StatusBadge>
              {task.error && <span className="text-sm text-muted-foreground">{task.error}</span>}
            </div>
          )}

          {logs.isLoading && <Spinner size="sm" />}

          {!logs.isLoading && !logs.data && <EmptyState variant="compact" title={t('dialogs.taskLogs.empty')} />}

          {logs.data && (
            <pre className="max-h-[55vh] overflow-auto rounded-lg bg-muted p-3 font-mono text-xs text-foreground">
              {logs.data}
            </pre>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
