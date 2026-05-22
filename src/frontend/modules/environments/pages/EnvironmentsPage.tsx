// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { AgentTokenDialog } from '../components/AgentTokenDialog';
import { CreateEnvironmentDialog } from '../components/CreateEnvironmentDialog';
import { useEnvironmentColumns } from '../components/EnvironmentsColumns';
import {
  useEnvironmentList,
  useTestEnvironment,
  useRemoveEnvironment,
} from '../hooks/useEnvironmentActions';
import type { InstallTokenResponse } from '../hooks/useEnvironmentActions';

type AgentTokenData = {
  token: string;
  installScript?: InstallTokenResponse | null;
};

export default function EnvironmentsPage() {
  const { t } = useTranslation('environments');
  const { data: environments = [], isLoading } = useEnvironmentList();
  const testEnv = useTestEnvironment();
  const removeEnv = useRemoveEnvironment();

  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [agentTokenData, setAgentTokenData] = useState<AgentTokenData | null>(null);

  const columns = useEnvironmentColumns({
    onTest: (id) => testEnv.mutate(id),
    onRemove: (id) => setConfirmTarget(id),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <IconPlus className="h-4 w-4" /> {t('addEnvironment')}
          </Button>
        }
      />

      <DataGrid
        data={environments}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('emptyMessage')}
      />

      <CreateEnvironmentDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onAgentToken={(token) => setAgentTokenData((prev) => ({ ...prev, token }))}
        onInstallScript={(data) => setAgentTokenData((prev) => prev ? { ...prev, installScript: data } : null)}
      />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) removeEnv.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={removeEnv.isPending}
      />

      {agentTokenData && (
        <AgentTokenDialog
          open={!!agentTokenData}
          onOpenChange={(open) => !open && setAgentTokenData(null)}
          token={agentTokenData.token}
          serverUrl={window.location.origin}
          installScript={agentTokenData.installScript}
        />
      )}
    </div>
  );
}
