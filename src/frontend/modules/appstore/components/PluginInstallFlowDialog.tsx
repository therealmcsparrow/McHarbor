// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PluginInstallDialog } from './PluginInstallDialog';
import { useInstallDialogTitle, type usePluginInstallFlow } from '../pages/usePluginInstallFlow';

interface PluginInstallFlowDialogProps {
  flow: ReturnType<typeof usePluginInstallFlow>;
}

export function PluginInstallFlowDialog({ flow }: PluginInstallFlowDialogProps) {
  const title = useInstallDialogTitle(flow);
  const pluginName = flow.pendingInstall?.entry.name ?? flow.installTarget?.name ?? flow.updateTargetName ?? '';

  return (
    <PluginInstallDialog
      title={title}
      pluginName={pluginName}
      open={flow.dialogOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen && !flow.stream.state.running) flow.closeInstallDialog();
      }}
      progress={flow.stream.state.progress}
      logs={flow.stream.state.logs}
      running={flow.stream.state.running}
      onAbort={flow.stream.abort}
      onClose={flow.closeInstallDialog}
      pendingInstall={flow.pendingInstall ?? undefined}
      selectedVersion={flow.selectedVersion}
      onSelectedVersionChange={flow.setSelectedVersion}
      onConfirmInstall={flow.confirmInstall}
      unsignedRefused={flow.unsignedRefused}
      onConfirmUnsigned={flow.confirmUnsigned}
    />
  );
}
