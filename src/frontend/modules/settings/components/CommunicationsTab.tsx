// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconMessageCircle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useNotificationChannels, type CommunicationChannel } from '../hooks/useNotificationChannels';
import { useEmailServers, type EmailServer } from '../hooks/useEmailServers';
import { ChannelCard } from './ChannelCard';
import { CreateChannelDialog } from './CreateChannelDialog';
import { EditChannelDialog } from './EditChannelDialog';
import { TestChannelDialog } from './TestChannelDialog';
import { EmailServerCard } from './EmailServerCard';
import { EditEmailServerDialog } from './EditEmailServerDialog';
import { TestEmailDialog } from './TestEmailDialog';

type CommunicationListItem =
  | { kind: 'email'; id: string; createdAt: string; server: EmailServer }
  | { kind: 'channel'; id: string; createdAt: string; channel: CommunicationChannel };

export function CommunicationsTab() {
  const { t } = useTranslation('settings');
  const { data: channels, isLoading: channelsLoading } = useNotificationChannels();
  const { data: servers, isLoading: serversLoading } = useEmailServers();
  const [createOpen, setCreateOpen] = useState(false);
  const [editChannel, setEditChannel] = useState<CommunicationChannel | null>(null);
  const [testChannel, setTestChannel] = useState<CommunicationChannel | null>(null);
  const [editServer, setEditServer] = useState<EmailServer | null>(null);
  const [testServer, setTestServer] = useState<EmailServer | null>(null);

  if (channelsLoading || serversLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  const items: CommunicationListItem[] = [
    ...(servers ?? []).map((server) => ({
      kind: 'email' as const,
      id: server.id,
      createdAt: server.createdAt,
      server,
    })),
    ...(channels ?? []).map((channel) => ({
      kind: 'channel' as const,
      id: channel.id,
      createdAt: channel.createdAt,
      channel,
    })),
  ].sort((a, b) => b.createdAt.localeCompare(a.createdAt));

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{t('communications.description')}</p>
        <Button onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-2 size-4" />
          {t('communications.addChannel')}
        </Button>
      </div>

      {items.length > 0 ? (
        <div className="space-y-3">
          {items.map((item) => {
            if (item.kind === 'email') {
              return (
                <EmailServerCard
                  key={`email-${item.id}`}
                  server={item.server}
                  onEdit={setEditServer}
                  onTest={setTestServer}
                />
              );
            }

            return (
              <ChannelCard
                key={`channel-${item.id}`}
                channel={item.channel}
                onEdit={setEditChannel}
                onTest={setTestChannel}
              />
            );
          })}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-10">
          <IconMessageCircle className="size-10 text-muted-foreground" />
          <p className="mt-3 text-sm text-muted-foreground">{t('communications.noChannels')}</p>
        </div>
      )}

      <CreateChannelDialog open={createOpen} onOpenChange={setCreateOpen} />

      {editChannel && (
        <EditChannelDialog
          open={!!editChannel}
          onOpenChange={(open) => { if (!open) setEditChannel(null); }}
          channel={editChannel}
        />
      )}

      {testChannel && (
        <TestChannelDialog
          open={!!testChannel}
          onOpenChange={(open) => { if (!open) setTestChannel(null); }}
          channelId={testChannel.id}
          channelName={testChannel.name}
        />
      )}

      {editServer && (
        <EditEmailServerDialog
          open={!!editServer}
          onOpenChange={(open) => { if (!open) setEditServer(null); }}
          server={editServer}
        />
      )}

      {testServer && (
        <TestEmailDialog
          open={!!testServer}
          onOpenChange={(open) => { if (!open) setTestServer(null); }}
          serverId={testServer.id}
          serverName={testServer.name}
        />
      )}
    </div>
  );
}
