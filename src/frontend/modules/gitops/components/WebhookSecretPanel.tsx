// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconKey, IconRefresh, IconTrash } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { StatusError } from '@resources/components/ui/StatusError';
import { useGenerateWebhookSecret, useRemoveWebhookSecret, useWebhookSecretStatus } from '../hooks/useWebhookSecret';
import { WebhookSecretRevealDialog } from './WebhookSecretRevealDialog';

type Repo = { id: string; name: string };

type WebhookSecretPanelProps = {
  repos: Repo[];
};

export function WebhookSecretPanel({ repos }: WebhookSecretPanelProps) {
  const { t } = useTranslation('gitops');
  if (repos.length === 0) return null;

  return (
    <div>
      <h3 className="text-sm font-semibold text-foreground">{t('webhookSecret.title')}</h3>
      <p className="mb-2 text-xs text-muted-foreground">{t('webhookSecret.description')}</p>
      <div className="space-y-2">
        {repos.map((repo) => (
          <WebhookSecretRow key={repo.id} repo={repo} />
        ))}
      </div>
    </div>
  );
}

function WebhookSecretRow({ repo }: { repo: Repo }) {
  const { t } = useTranslation('gitops');
  const secretStatus = useWebhookSecretStatus(repo.id);
  const generate = useGenerateWebhookSecret(repo.id, t);
  const remove = useRemoveWebhookSecret(repo.id, t);
  const [secret, setSecret] = useState<string | null>(null);
  const [pending, setPending] = useState<'rotate' | 'remove' | null>(null);

  const configured = secretStatus.data?.configured ?? false;

  const mint = () => {
    generate.mutate(undefined, {
      onSuccess: (data) => {
        setSecret(data.secret);
        setPending(null);
        generate.reset();
      },
    });
  };

  const drop = () => {
    remove.mutate(undefined, { onSuccess: () => setPending(null) });
  };

  const closeConfirm = (isOpen: boolean) => {
    if (isOpen) return;
    setPending(null);
    generate.reset();
    remove.reset();
  };

  const failure = pending === 'rotate' ? generate.error : remove.error;

  return (
    <div className="mc-glass flex items-center justify-between gap-2 rounded-md px-3 py-2 text-sm">
      <span className="flex min-w-0 items-center gap-2 font-medium">
        <IconKey className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="truncate">{repo.name}</span>
      </span>
      <div className="flex shrink-0 items-center gap-2">
        {secretStatus.isError && <StatusError error={secretStatus.error.message} />}
        {secretStatus.data && (
          <Badge variant={configured ? 'success' : 'secondary'}>
            {configured ? t('webhookSecret.configured') : t('webhookSecret.notConfigured')}
          </Badge>
        )}
        <Button
          permission={PERM.GITOPS_MANAGE}
          variant="outline"
          size="sm"
          disabled={generate.isPending}
          aria-label={t(configured ? 'webhookSecret.rotateFor' : 'webhookSecret.generateFor', { repo: repo.name })}
          onClick={configured ? () => setPending('rotate') : mint}
        >
          <IconRefresh className="mr-1.5 size-3.5" />
          {configured ? t('webhookSecret.rotate') : t('webhookSecret.generate')}
        </Button>
        {configured && (
          <Button
            permission={PERM.GITOPS_MANAGE}
            variant="ghost"
            size="icon"
            aria-label={t('webhookSecret.removeFor', { repo: repo.name })}
            onClick={() => setPending('remove')}
          >
            <IconTrash className="size-4" />
          </Button>
        )}
      </div>

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={closeConfirm}
        variant={pending === 'remove' ? 'destructive' : 'caution'}
        permission={PERM.GITOPS_MANAGE}
        title={t(pending === 'remove' ? 'webhookSecret.removeTitle' : 'webhookSecret.rotateTitle', { repo: repo.name })}
        description={t(pending === 'remove' ? 'webhookSecret.removeDescription' : 'webhookSecret.rotateDescription')}
        confirmLabel={t(pending === 'remove' ? 'webhookSecret.remove' : 'webhookSecret.rotate')}
        loading={generate.isPending || remove.isPending}
        onConfirm={pending === 'remove' ? drop : mint}
      >
        {failure && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
          >
            {failure.message}
          </p>
        )}
      </ConfirmDialog>

      <WebhookSecretRevealDialog repoName={repo.name} secret={secret} onClose={() => setSecret(null)} />
    </div>
  );
}
