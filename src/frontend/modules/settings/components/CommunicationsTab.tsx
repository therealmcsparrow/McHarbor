// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconMessageCircle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Stack } from '@resources/components/ui/Layout';
import { SettingsShell } from '@resources/components/ui/SettingsShell';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useNotificationAdapters } from '../hooks/useCommunicationAdapters';
import { useNotificationChannels, type CommunicationChannel } from '../hooks/useNotificationChannels';
import { ChannelCard } from './ChannelCard';
import { CreateChannelDialog } from './CreateChannelDialog';
import { EditChannelDialog } from './EditChannelDialog';
import { TestChannelDialog } from './TestChannelDialog';

/** Settings → Communications. Mirrors AIProvidersTab: a SettingsShell
 *  header with the Add button, a card list, and one EmptyState that
 *  says *why* the list is empty — no plugin installed (go to the
 *  store) versus no channel configured yet (press Add). Channel
 *  transports are plugin-owned, so the two cases need different fixes. */
export function CommunicationsTab() {
  const { t } = useTranslation('settings');
  const navigate = useNavigate();
  const { data: channels, isLoading: channelsLoading } = useNotificationChannels();
  const { data: adapters, isLoading: adaptersLoading } = useNotificationAdapters();
  const [createOpen, setCreateOpen] = useState(false);
  const [editChannel, setEditChannel] = useState<CommunicationChannel | null>(null);
  const [testChannel, setTestChannel] = useState<CommunicationChannel | null>(null);

  const items = useMemo(() => [...(channels ?? [])].sort((a, b) => b.createdAt.localeCompare(a.createdAt)), [channels]);
  const anyPluginInstalled = Object.keys(adapters ?? {}).length > 0;
  const isLoading = channelsLoading || adaptersLoading;

  return (
    <SettingsShell
      title={t('communications.title')}
      description={t('communications.description')}
      headerActions={
        <Button permission={PERM.COMMUNICATIONS_MANAGE} onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-2 size-4" />
          {t('communications.addChannel')}
        </Button>
      }
    >
      {isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner />
        </div>
      ) : items.length > 0 ? (
        <Stack gap={3}>
          {items.map((channel) => (
            <ChannelCard key={channel.id} channel={channel} onEdit={setEditChannel} onTest={setTestChannel} />
          ))}
        </Stack>
      ) : (
        <EmptyState
          icon={<IconMessageCircle className="size-6" />}
          title={anyPluginInstalled ? t('communications.emptyTitle') : t('communications.noPluginsInstalledTitle')}
          description={
            anyPluginInstalled
              ? t('communications.emptyDescription')
              : t('communications.noPluginsInstalledDescription')
          }
          primaryAction={
            anyPluginInstalled
              ? { label: t('communications.addChannel'), onClick: () => setCreateOpen(true) }
              : {
                  label: t('communications.openAppStore'),
                  onClick: () => navigate('/store?tab=plugins'),
                }
          }
        />
      )}

      <CreateChannelDialog open={createOpen} onOpenChange={setCreateOpen} />

      {editChannel && (
        <EditChannelDialog
          open={!!editChannel}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) setEditChannel(null);
          }}
          channel={editChannel}
        />
      )}

      {testChannel && (
        <TestChannelDialog
          open={!!testChannel}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) setTestChannel(null);
          }}
          channelId={testChannel.id}
          channelName={testChannel.name}
        />
      )}
    </SettingsShell>
  );
}
