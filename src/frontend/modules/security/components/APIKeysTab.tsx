// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useAPIKeys, useRevokeAPIKey, type CreateAPIKeyResult } from '../hooks/useAPIKeys';
import { timeAgo } from '@resources/utils/format';
import { CreateAPIKeyDialog, APIKeyTokenDialog } from './CreateAPIKeyDialog';

export function APIKeysTab() {
  const { t } = useTranslation('security');
  const { data: keys, isLoading } = useAPIKeys();
  const [createOpen, setCreateOpen] = useState(false);
  const [revokeId, setRevokeId] = useState<string | null>(null);
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResult | null>(null);
  const revokeKey = useRevokeAPIKey();

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{t('apiKeys.description')}</p>
        <Button variant="outline" size="sm" onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-1 size-3.5" />
          {t('apiKeys.createKey')}
        </Button>
      </div>

      {keys && keys.length > 0 ? (
        <div className="rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('apiKeys.name')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('apiKeys.key')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('apiKeys.expiresAt')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('apiKeys.lastUsed')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('apiKeys.status')}</th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {keys.map((key) => {
                const isExpired = key.expiresAt && new Date(key.expiresAt) < new Date();
                return (
                  <tr key={key.id} className="border-b border-border last:border-0">
                    <td className="px-4 py-2">
                      <div className="font-medium">{key.name}</div>
                      <div className="text-xs text-muted-foreground">{key.username}</div>
                    </td>
                    <td className="px-4 py-2">
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{key.keyPrefix}</code>
                    </td>
                    <td className="px-4 py-2 text-muted-foreground">
                      {key.expiresAt ? timeAgo(key.expiresAt) : t('apiKeys.noExpiry')}
                    </td>
                    <td className="px-4 py-2 text-muted-foreground">
                      {key.lastUsedAt ? timeAgo(key.lastUsedAt) : t('apiKeys.never')}
                    </td>
                    <td className="px-4 py-2">
                      <Badge variant={!key.isActive ? 'destructive' : isExpired ? 'warning' : 'success'}>
                        {!key.isActive ? t('apiKeys.revoked') : isExpired ? t('apiKeys.expired') : t('apiKeys.active')}
                      </Badge>
                    </td>
                    <td className="px-4 py-2 text-right">
                      {key.isActive && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setRevokeId(key.id)}
                          aria-label={t('apiKeys.revokeKey')}
                        >
                          <IconTrash className="size-3.5" />
                        </Button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
          {t('apiKeys.noKeys')}
        </div>
      )}

      <CreateAPIKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(result) => {
          setCreateOpen(false);
          setCreatedKey(result);
        }}
      />

      {createdKey && (
        <APIKeyTokenDialog
          open={!!createdKey}
          onOpenChange={() => setCreatedKey(null)}
          keyResult={createdKey}
        />
      )}

      <ConfirmDialog
        open={!!revokeId}
        onOpenChange={() => setRevokeId(null)}
        title={t('apiKeys.revokeKey')}
        description={t('apiKeys.revokeKeyConfirm')}
        onConfirm={() => {
          if (revokeId) {
            revokeKey.mutate(revokeId, { onSuccess: () => setRevokeId(null) });
          }
        }}
        loading={revokeKey.isPending}
      />
    </div>
  );
}
