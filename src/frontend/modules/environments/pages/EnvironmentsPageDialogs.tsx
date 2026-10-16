// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The four dialogs the environments list can raise: create, remove
// confirmation, the one-shot agent token, and the connection test. Split
// out of EnvironmentsPage.tsx unchanged.

import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { AgentTokenDialog } from '../components/AgentTokenDialog';
import { CreateEnvironmentDialog } from '../components/CreateEnvironmentDialog';
import { EnvironmentTestDialog } from '../components/EnvironmentTestDialog';
import type { AgentTokenData } from './environments-page-types';

type EnvironmentsPageDialogsProps = {
  createOpen: boolean;
  setCreateOpen: (open: boolean) => void;
  setAgentTokenData: React.Dispatch<React.SetStateAction<AgentTokenData | null>>;
  confirmTarget: string | null;
  setConfirmTarget: (id: string | null) => void;
  onConfirmRemove: (id: string) => void;
  removePending: boolean;
  agentTokenData: AgentTokenData | null;
  testTarget: { id: string; name: string } | null;
  setTestTarget: (target: { id: string; name: string } | null) => void;
  testResult: React.ComponentProps<typeof EnvironmentTestDialog>['result'];
  testPending: boolean;
};

export function EnvironmentsPageDialogs({
  createOpen,
  setCreateOpen,
  setAgentTokenData,
  confirmTarget,
  setConfirmTarget,
  onConfirmRemove,
  removePending,
  agentTokenData,
  testTarget,
  setTestTarget,
  testResult,
  testPending,
}: EnvironmentsPageDialogsProps) {
  const { t } = useTranslation('environments');
  return (
    <>
      <CreateEnvironmentDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onAgentToken={(token) => setAgentTokenData((prev) => ({ ...prev, token }))}
        onInstallScript={(data) => setAgentTokenData((prev) => (prev ? { ...prev, installScript: data } : null))}
      />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(next) => !next && setConfirmTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) onConfirmRemove(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={removePending}
      />

      {agentTokenData && (
        <AgentTokenDialog
          open={!!agentTokenData}
          onOpenChange={(next) => !next && setAgentTokenData(null)}
          token={agentTokenData.token}
          serverUrl={window.location.origin}
          installScript={agentTokenData.installScript}
          mode={agentTokenData.mode}
        />
      )}

      <EnvironmentTestDialog
        open={testTarget !== null}
        onOpenChange={(next) => {
          if (!next) {
            setTestTarget(null);
          }
        }}
        result={testResult}
        pending={testPending}
        envName={testTarget?.name ?? ''}
      />
    </>
  );
}
