// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// APIKeyDetailHero — the identity half of the API-key detail card.
//
// Same shape as UserDetailHero: a tinted tile, an <h1> naming the key, and a
// metadata row. The tile takes its tone from the key's state, so "this
// credential no longer works" is legible before a word is read.
//
// There are deliberately no actions here. Editing and revoking live on the
// list, and a deleted key has nothing left to act on — putting a disabled
// row of buttons on a tombstone would only invite the question of why.

import { useTranslation } from 'react-i18next';
import { IconKey, IconUser } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import { timeAgo } from '@resources/utils/format';
import type { APIKeyDetail, APIKeyStatus } from '../hooks/useAPIKeys';

type APIKeyDetailHeroProps = {
  detail: APIKeyDetail;
};

const STATUS_TONE: Record<APIKeyStatus, string> = {
  active: 'bg-primary/12 text-primary ring-primary/25',
  expired: 'bg-warning/12 text-warning ring-warning/25',
  revoked: 'bg-destructive/12 text-destructive ring-destructive/25',
  deleted: 'bg-muted text-muted-foreground ring-border',
};

const STATUS_BADGE: Record<APIKeyStatus, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  active: 'success',
  expired: 'warning',
  revoked: 'destructive',
  deleted: 'secondary',
};

export function APIKeyDetailHero({ detail }: APIKeyDetailHeroProps) {
  const { t } = useTranslation('security');

  return (
    <div className="flex flex-1 flex-col gap-2.5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className={cn(
              'grid size-11 shrink-0 place-items-center rounded-xl ring-1 ring-inset',
              STATUS_TONE[detail.status],
            )}
          >
            <IconKey className="size-5" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
              {detail.name}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1">
              <Badge variant={STATUS_BADGE[detail.status]}>{t(`apiKeys.${detail.status}`)}</Badge>
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{detail.keyPrefix}</code>
              <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                <IconUser className="size-3 shrink-0" aria-hidden="true" />
                <span className="truncate">{detail.username}</span>
              </span>
              <span className="text-xs text-muted-foreground">
                {t('apiKeys.lastUsed')}: {detail.lastUsedAt ? timeAgo(detail.lastUsedAt) : t('apiKeys.never')}
              </span>
              <span className="text-xs text-muted-foreground">
                {t('apiKeys.expiresAt')}: {detail.expiresAt ? timeAgo(detail.expiresAt) : t('apiKeys.noExpiry')}
              </span>
              {detail.deletedAt && (
                <span className="text-xs text-muted-foreground">
                  {t('apiKeys.detail.deletedAt')}: {timeAgo(detail.deletedAt)}
                </span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
