// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconMail } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useEmailServers, type EmailServer } from '../hooks/useEmailServers';
import { EmailServerCard } from './EmailServerCard';
import { CreateEmailServerDialog } from './CreateEmailServerDialog';
import { EditEmailServerDialog } from './EditEmailServerDialog';
import { TestEmailDialog } from './TestEmailDialog';

export function EmailTab() {
  const { t } = useTranslation('settings');
  const { data: servers, isLoading } = useEmailServers();
  const [createOpen, setCreateOpen] = useState(false);
  const [editServer, setEditServer] = useState<EmailServer | null>(null);
  const [testServer, setTestServer] = useState<EmailServer | null>(null);

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
        <p className="text-sm text-muted-foreground">{t('email.description')}</p>
        <Button onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-2 size-4" />
          {t('email.addServer')}
        </Button>
      </div>

      {servers && servers.length > 0 ? (
        <div className="space-y-3">
          {servers.map((server) => (
            <EmailServerCard
              key={server.id}
              server={server}
              onEdit={setEditServer}
              onTest={setTestServer}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <IconMail className="size-10 text-muted-foreground" />
          <p className="mt-3 text-sm text-muted-foreground">{t('email.noServers')}</p>
        </div>
      )}

      <CreateEmailServerDialog open={createOpen} onOpenChange={setCreateOpen} />

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
