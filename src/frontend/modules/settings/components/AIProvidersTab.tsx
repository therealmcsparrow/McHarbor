// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { IconBolt, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Stack } from '@resources/components/ui/Layout';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { SettingsShell } from '@resources/components/ui/SettingsShell';
import { usePluginInstalled } from '@core/plugins/loader';
import { useAIProviderConnections, type AIProviderConnection } from '../hooks/useAIProviderConnections';
import { useAIProviderAdapters, aiAdapterQueryKey } from '../hooks/useAIProviderAdapters';
import { AIProviderConnectionCard } from './AIProviderConnectionCard';
import { CreateAIProviderConnectionDialog } from './CreateAIProviderConnectionDialog';
import { EditAIProviderConnectionDialog } from './EditAIProviderConnectionDialog';
import { TestAIProviderConnectionDialog } from './TestAIProviderConnectionDialog';
import { PERM } from '@core/auth/permissionKeys';

/** Lists the AI provider connections stored in `ai_provider_connections`.
 *  After the "AI Providers as Plugins → Named connections" refactor
 *  (2026-08-04) this tab is the host-owned CRUD for the *instances*
 *  the operator can name, enable, mark as default, and test. The
 *  underlying *plugins* are still installed from the App Store; the
 *  install-aware picker in CreateAIProviderConnectionDialog bridges
 *  the two surfaces. */
// The backend accepts settings.manage OR ai_assistant.manage on every
// /ai-providers route (ai_providers/routes.go). Gating the controls on
// settings.manage alone made the UI stricter than the check behind it: a
// user holding only ai_assistant.manage saw everything disabled.
const AI_PROVIDER_MANAGE = [PERM.SETTINGS_MANAGE, PERM.AI_ASSISTANT_MANAGE];

export function AIProvidersTab() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  const connectionsQuery = useAIProviderConnections();
  const adaptersQuery = useAIProviderAdapters();
  const { data: connections, isLoading: connectionsLoading } = connectionsQuery;
  const { data: aiAdapters, isLoading: adaptersLoading } = adaptersQuery;
  const installedPlugins = usePluginInstalled();
  const [createOpen, setCreateOpen] = useState(false);
  const [editConnection, setEditConnection] = useState<AIProviderConnection | null>(null);
  const [testConnection, setTestConnection] = useState<AIProviderConnection | null>(null);

  // Build a ProviderType → DisplayName lookup from the runtime
  // adapter catalog. Cards fall back to the raw ProviderType when
  // the catalog hasn't loaded yet so the tab still renders.
  const providerDisplayNames = useMemo(() => {
    const out = new Map<string, string>();
    if (aiAdapters) {
      for (const [type, spec] of Object.entries(aiAdapters)) {
        out.set(type, spec.DisplayName || type);
      }
    }
    return out;
  }, [aiAdapters]);

  function handleOpenCreate() {
    void queryClient.invalidateQueries({ queryKey: aiAdapterQueryKey });
    setCreateOpen(true);
  }

  const isLoading = connectionsLoading || adaptersLoading;
  const sorted = useMemo(() => {
    return [...(connections ?? [])].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
  }, [connections]);

  const anyPluginInstalled = installedPlugins.some((plugin) => plugin.slug.startsWith('ai-provider-'));

  return (
    <SettingsShell
      title={t('aiProviders.title')}
      description={t('aiProviders.description')}
      headerActions={
        <Button permission={AI_PROVIDER_MANAGE} onClick={handleOpenCreate}>
          <IconPlus className="mr-2 size-4" />
          {t('aiProviders.addProvider')}
        </Button>
      }
    >
      {connectionsQuery.isError || adaptersQuery.isError ? (
        <ErrorState
          variant="card"
          error={connectionsQuery.error ?? adaptersQuery.error}
          onRetry={() => {
            void connectionsQuery.refetch();
            void adaptersQuery.refetch();
          }}
        />
      ) : isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner />
        </div>
      ) : sorted.length > 0 ? (
        <Stack gap={3}>
          {sorted.map((connection) => (
            <AIProviderConnectionCard
              key={connection.id}
              connection={connection}
              providerDisplayName={providerDisplayNames.get(connection.providerType) ?? connection.providerType}
              providerSpec={aiAdapters?.[connection.providerType]}
              onEdit={setEditConnection}
              onTest={setTestConnection}
            />
          ))}
        </Stack>
      ) : (
        <EmptyState
          icon={<IconBolt className="size-6" />}
          title={anyPluginInstalled ? t('aiProviders.emptyTitle') : t('aiProviders.noPluginsInstalledTitle')}
          description={
            anyPluginInstalled ? t('aiProviders.emptyDescription') : t('aiProviders.noPluginsInstalledDescription')
          }
          primaryAction={
            anyPluginInstalled
              ? {
                  label: t('aiProviders.addProvider'),
                  onClick: handleOpenCreate,
                }
              : {
                  label: t('aiProviders.openAppStore'),
                  onClick: () => {
                    window.location.href = '/store?tab=plugins';
                  },
                }
          }
        />
      )}

      <CreateAIProviderConnectionDialog open={createOpen} onOpenChange={setCreateOpen} />

      {editConnection && (
        <EditAIProviderConnectionDialog
          open={!!editConnection}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) setEditConnection(null);
          }}
          connection={editConnection}
        />
      )}

      {testConnection && (
        <TestAIProviderConnectionDialog
          open={!!testConnection}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) setTestConnection(null);
          }}
          connectionId={testConnection.id}
          connectionName={testConnection.name}
        />
      )}
    </SettingsShell>
  );
}
