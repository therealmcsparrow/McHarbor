// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconCloudLock, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Stack } from '@resources/components/ui/Layout';
import { SettingsShell } from '@resources/components/ui/SettingsShell';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useLogForwarderAdapters, useLogForwarders, type LogForwarder } from '../hooks/useLogForwarders';
import { LogForwarderCard } from './LogForwarderCard';
import { LogForwarderDialog } from './LogForwarderDialog';

/**
 * Settings → Integrations → Logging: where the audit trail is shipped.
 *
 * Mirrors AIProvidersTab: a SettingsShell header with the Add button, a
 * card list, and one EmptyState whose message depends on *why* it is
 * empty. Nothing installed is a store problem; nothing configured is an
 * Add-button problem, and telling an operator to go to the store when
 * the plugin is already installed would send them the wrong way.
 *
 * A forwarder is a *named destination*, not a type. One SIEM adapter can
 * back a production collector and a staging one, so the Add button never
 * runs out of things to add while an adapter is installed. Forwarders
 * configured before their plugin was removed still list, so an operator
 * can see and clean up what stopped.
 */
export function LoggingTab() {
  const { t } = useTranslation('settings');
  const navigate = useNavigate();
  const { data: adapters = [], isLoading: adaptersLoading } = useLogForwarderAdapters();
  const { data: forwarders = [], isLoading } = useLogForwarders();

  const [createOpen, setCreateOpen] = useState(false);
  const [editForwarder, setEditForwarder] = useState<LogForwarder | null>(null);

  const anyPluginInstalled = adapters.length > 0;

  return (
    <SettingsShell
      title={t('logging.title')}
      description={t('logging.description')}
      headerActions={
        <Button
          permission={PERM.SETTINGS_MANAGE}
          onClick={() => setCreateOpen(true)}
          disabled={!anyPluginInstalled}
          title={anyPluginInstalled ? undefined : t('logging.noAdapters')}
        >
          <IconPlus className="mr-2 size-4" />
          {t('logging.addForwarder')}
        </Button>
      }
    >
      {isLoading || adaptersLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner />
        </div>
      ) : forwarders.length > 0 ? (
        <Stack gap={3}>
          {forwarders.map((forwarder) => (
            <LogForwarderCard key={forwarder.id} forwarder={forwarder} onEdit={setEditForwarder} />
          ))}
        </Stack>
      ) : (
        <EmptyState
          icon={<IconCloudLock className="size-6" />}
          title={anyPluginInstalled ? t('logging.emptyTitle') : t('logging.noPluginsInstalledTitle')}
          description={anyPluginInstalled ? t('logging.emptyDescription') : t('logging.noPluginsInstalledDescription')}
          primaryAction={
            anyPluginInstalled
              ? { label: t('logging.addForwarder'), onClick: () => setCreateOpen(true) }
              : {
                  label: t('logging.openAppStore'),
                  onClick: () => navigate('/store?tab=plugins'),
                }
          }
        />
      )}

      <LogForwarderDialog open={createOpen} onOpenChange={setCreateOpen} adapters={adapters} />
      {editForwarder && (
        <LogForwarderDialog
          open={!!editForwarder}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) setEditForwarder(null);
          }}
          adapters={adapters}
          forwarder={editForwarder}
        />
      )}
    </SettingsShell>
  );
}
