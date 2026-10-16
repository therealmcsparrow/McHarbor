// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { useEnvironmentDetailState } from '../hooks/useEnvironmentDetailState';
import type { useTestEnvironment } from '../hooks/useEnvironmentActions';
import type { EnvironmentInfo } from '../hooks/useEnvironments';
import { AgentTokenDialog } from './AgentTokenDialog';
import { EditEnvironmentDialog } from './EditEnvironmentDialog';
import { EnvironmentTestDialog } from './EnvironmentTestDialog';
import { MigrateConnectionDialog } from './MigrateConnectionDialog';

type EnvironmentDetailDialogsProps = {
  env: EnvironmentInfo;
  state: ReturnType<typeof useEnvironmentDetailState>;
  testEnv: ReturnType<typeof useTestEnvironment>;
  migrateOpen: boolean;
  onMigrateOpenChange: (open: boolean) => void;
  editOpen: boolean;
  onEditOpenChange: (open: boolean) => void;
  testOpen: boolean;
  onTestOpenChange: (open: boolean) => void;
};

export function EnvironmentDetailDialogs({
  env,
  state,
  testEnv,
  migrateOpen,
  onMigrateOpenChange,
  editOpen,
  onEditOpenChange,
  testOpen,
  onTestOpenChange,
}: EnvironmentDetailDialogsProps) {
  return (
    <>
      <AgentTokenDialog
        open={state.tokenDialogOpen}
        onOpenChange={state.setTokenDialogOpen}
        token={state.regeneratedToken}
        caFingerprint={state.regeneratedPin}
        serverUrl={`${window.location.protocol}//${window.location.host}`}
      />
      <MigrateConnectionDialog
        open={migrateOpen}
        onOpenChange={onMigrateOpenChange}
        environment={env}
        onAgentToken={(token) => {
          state.setRegeneratedToken(token);
          state.setTokenDialogOpen(true);
        }}
      />
      <EditEnvironmentDialog open={editOpen} onOpenChange={onEditOpenChange} env={env} />

      <EnvironmentTestDialog
        open={testOpen}
        onOpenChange={(next) => {
          onTestOpenChange(next);
          if (!next) testEnv.reset();
        }}
        envName={env.name}
        pending={testEnv.isPending}
        result={testEnv.data}
      />
    </>
  );
}
