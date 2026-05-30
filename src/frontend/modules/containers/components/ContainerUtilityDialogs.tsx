// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconFileText, IconTerminal2 } from '@tabler/icons-react';
import { OperationProgressDialog } from '@resources/components/OperationProgressDialog';
import { TakeOverDialog } from '@resources/components/TakeOverDialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import type { ContainerInfo } from '@core/types/docker';
import type { BatchProgressDialogState } from '@resources/hooks/useBatchProgressOperation';
import { LogsModal } from './LogsModal';
import { RemoveContainerDialog } from './RemoveContainerDialog';
import { RenameContainerDialog } from './RenameContainerDialog';
import { TerminalModal } from './TerminalModal';

type ContainerUtilityDialogsProps = {
  removeTarget: ContainerInfo | null;
  renameTarget: ContainerInfo | null;
  terminalTarget: ContainerInfo | null;
  logsTarget: ContainerInfo | null;
  takeOverTarget: ContainerInfo | null;
  progressState: BatchProgressDialogState;
  closeProgress: () => void;
  setRemoveTarget: (container: ContainerInfo | null) => void;
  setRenameTarget: (container: ContainerInfo | null) => void;
  setTerminalTarget: (container: ContainerInfo | null) => void;
  setLogsTarget: (container: ContainerInfo | null) => void;
  setTakeOverTarget: (container: ContainerInfo | null) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

export function ContainerUtilityDialogs({
  removeTarget,
  renameTarget,
  terminalTarget,
  logsTarget,
  takeOverTarget,
  progressState,
  closeProgress,
  setRemoveTarget,
  setRenameTarget,
  setTerminalTarget,
  setLogsTarget,
  setTakeOverTarget,
  t,
}: ContainerUtilityDialogsProps) {
  return (
    <>
      <RenameContainerDialog
        container={
          renameTarget
            ? {
                id: renameTarget.Id,
                name: renameTarget.Names?.[0]?.replace(/^\//, '') ?? renameTarget.Id,
              }
            : null
        }
        open={renameTarget !== null}
        onOpenChange={(open) => !open && setRenameTarget(null)}
      />

      <RemoveContainerDialog
        container={
          removeTarget
            ? {
                id: removeTarget.Id,
                name: removeTarget.Names?.[0]?.replace(/^\//, '') ?? removeTarget.Id,
                image: removeTarget.Image,
                imageId: removeTarget.ImageID,
                stackName: removeTarget.StackName ?? removeTarget.Labels?.['com.docker.compose.project'] ?? null,
              }
            : null
        }
        open={removeTarget !== null}
        onOpenChange={(open) => !open && setRemoveTarget(null)}
      />

      <Dialog open={terminalTarget !== null} onOpenChange={(open) => !open && setTerminalTarget(null)}>
        <DialogContent className="max-w-4xl p-0">
          <DialogHeader className="px-4 py-3">
            <DialogTitle className="flex items-center gap-2 text-sm">
              <IconTerminal2 className="h-4 w-4 text-violet-400" />
              {t('terminal.title', { name: terminalTarget?.Names?.[0]?.replace(/^\//, '') ?? '' })}
            </DialogTitle>
            <DialogDescription className="sr-only">{t('terminal.description')}</DialogDescription>
          </DialogHeader>
          {terminalTarget && (
            <DialogBody className="p-0">
              <TerminalModal containerId={terminalTarget.Id} />
            </DialogBody>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={logsTarget !== null} onOpenChange={(open) => !open && setLogsTarget(null)}>
        <DialogContent className="max-w-4xl p-0">
          <DialogHeader className="px-4 py-3">
            <DialogTitle className="flex items-center gap-2 text-sm">
              <IconFileText className="h-4 w-4 text-cyan-400" />
              {t('logs.title', { name: logsTarget?.Names?.[0]?.replace(/^\//, '') ?? '' })}
            </DialogTitle>
            <DialogDescription className="sr-only">{t('logs.description')}</DialogDescription>
          </DialogHeader>
          {logsTarget && (
            <DialogBody className="p-0">
              <LogsModal containerId={logsTarget.Id} isRunning={logsTarget.State === 'running'} />
            </DialogBody>
          )}
        </DialogContent>
      </Dialog>

      <TakeOverDialog
        open={takeOverTarget !== null}
        onOpenChange={(open) => !open && setTakeOverTarget(null)}
        containerId={takeOverTarget?.Id}
        containerName={takeOverTarget?.Names?.[0]?.replace(/^\//, '')}
      />

      <OperationProgressDialog state={progressState} onClose={closeProgress} />
    </>
  );
}
