// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconMessageCircle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useNotificationChannels, type CommunicationChannel } from '../hooks/useNotificationChannels';
import { ChannelCard } from './ChannelCard';
import { CreateChannelDialog } from './CreateChannelDialog';
import { EditChannelDialog } from './EditChannelDialog';
import { TestChannelDialog } from './TestChannelDialog';

export function CommunicationsTab() {
  const { t } = useTranslation('settings');
  const { data: channels, isLoading } = useNotificationChannels();
  const [createOpen, setCreateOpen] = useState(false);
  const [editChannel, setEditChannel] = useState<CommunicationChannel | null>(null);
  const [testChannel, setTestChannel] = useState<CommunicationChannel | null>(null);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{t('communications.description')}</p>
        <Button onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-2 size-4" />
          {t('communications.addChannel')}
        </Button>
      </div>

      {channels && channels.length > 0 ? (
        <div className="space-y-3">
          {channels.map((channel) => (
            <ChannelCard
              key={channel.id}
              channel={channel}
              onEdit={setEditChannel}
              onTest={setTestChannel}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
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
    </div>
  );
}
