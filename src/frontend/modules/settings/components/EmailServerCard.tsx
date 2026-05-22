// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconMail,
  IconBrandWindows,
  IconBrandGoogle,
  IconTrash,
  IconPencil,
  IconSend,
  IconStar,
  IconStarFilled,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useUpdateEmailServer,
  useDeleteEmailServer,
  useSetDefaultEmailServer,
  type EmailServer,
} from '../hooks/useEmailServers';

type EmailServerCardProps = {
  server: EmailServer;
  onEdit: (server: EmailServer) => void;
  onTest: (server: EmailServer) => void;
};

const TYPE_CONFIG = {
  smtp: { icon: IconMail, color: 'text-primary', label: 'SMTP' },
  exchange: { icon: IconBrandWindows, color: 'text-[#0078D4]', label: 'Exchange Online' },
  gmail: { icon: IconBrandGoogle, color: 'text-[#4285F4]', label: 'Gmail' },
} as const;

export function EmailServerCard({ server, onEdit, onTest }: EmailServerCardProps) {
  const { t } = useTranslation('settings');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const updateServer = useUpdateEmailServer();
  const deleteServer = useDeleteEmailServer();
  const setDefault = useSetDefaultEmailServer();

  const config = TYPE_CONFIG[server.serverType];
  const Icon = config.icon;

  function handleToggle(enabled: boolean) {
    updateServer.mutate({ id: server.id, enabled });
  }

  function handleSetDefault() {
    setDefault.mutate(server.id);
  }

  function handleDelete() {
    deleteServer.mutate(server.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  return (
    <>
      <div className="flex items-center gap-4 rounded-lg border border-border bg-card p-4">
        <Icon className={`size-8 shrink-0 ${config.color}`} />

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">
              {server.name}
            </h4>
            {server.isDefault && (
              <Badge variant="default">{t('email.default')}</Badge>
            )}
            <Badge variant={server.enabled ? 'default' : 'secondary'}>
              {server.enabled ? t('email.enabled') : t('email.disabled')}
            </Badge>
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {config.label} &middot; {server.fromAddress}
          </p>
        </div>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={server.isDefault ? t('email.isDefault') : t('email.setDefault')}
            onClick={handleSetDefault}
            disabled={server.isDefault || setDefault.isPending}
          >
            {server.isDefault ? (
              <IconStarFilled className="size-4 text-amber-400" />
            ) : (
              <IconStar className="size-4" />
            )}
          </Button>
          <Switch
            checked={server.enabled}
            onCheckedChange={handleToggle}
            disabled={updateServer.isPending}
          />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('email.testServer')}
            onClick={() => onTest(server)}
          >
            <IconSend className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('email.editServer')}
            onClick={() => onEdit(server)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('email.deleteServer')}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('email.confirmDeleteTitle')}
        description={t('email.confirmDeleteDescription')}
        onConfirm={handleDelete}
        loading={deleteServer.isPending}
        variant="destructive"
      />
    </>
  );
}
