// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDeviceLaptop, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { Flex } from '@resources/components/ui/Layout';
import { Text as UiText } from '@resources/components/ui/Typography';
import {
  useUserSessions,
  useRevokeUserSession,
  useRevokeAllUserSessions,
  useRevokeMySession,
  useRevokeAllMySessions,
  type SessionView,
} from '@resources/hooks/useSessions';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';

type SessionsManagerProps = {
  /**
   * User id to manage. When `null` we use the self-service
   * endpoint (/auth/sessions) instead of the admin one. The `null`
   * case is used by the profile page; the security page passes
   * a real id.
   */
  userId: string | null;
  /** Hide the "revoke all" CTA. Useful when the caller is read-only. */
  hideBulkActions?: boolean;
  className?: string;
};

/**
 * SessionsManager renders the session list for a single user
 * (admin view) or the calling user (self view). Each row shows
 * the truncated handle, IP, user agent, creation time and an
 * action button to revoke the session. The current session is
 * marked with a badge and is not revocable from this UI (use
 * logout instead).
 */
export function SessionsManager({ userId, hideBulkActions, className }: SessionsManagerProps) {
  const { t } = useTranslation('security');
  const { data: sessions, isLoading } = useUserSessions(userId ?? null);
  const revoke = useRevokeUserSession();
  const revokeAll = useRevokeAllUserSessions();
  // Self view (userId === null) talks to /auth/sessions, which needs no
  // admin permission: revoking your own session is account self-management.
  // Until now the row action was simply disabled here, so a user could see
  // their sessions and do nothing about them.
  const revokeMine = useRevokeMySession();

  if (isLoading) {
    return (
      <Flex align="center" justify="center" className="h-32">
        <Spinner size="lg" />
      </Flex>
    );
  }

  if (!sessions || sessions.length === 0) {
    return <EmptyState variant="compact" icon={<IconDeviceLaptop className="size-5" />} title={t('sessions.empty')} />;
  }

  return (
    <div className={className}>
      <div className="mb-3 flex items-center justify-between">
        <UiText size="sm" tone="muted">
          {t('sessions.count', {
            count: sessions.length,
          })}
        </UiText>
        {!hideBulkActions && userId && (
          <Button
            permission={PERM.SESSIONS_MANAGE}
            variant="outline"
            size="sm"
            onClick={() => revokeAll.mutate({ userId })}
            disabled={revokeAll.isPending}
          >
            {t('sessions.revokeAll')}
          </Button>
        )}
        {!hideBulkActions && !userId && <SelfRevokeAllButton />}
      </div>
      <ul className="space-y-2">
        {sessions.map((s) => (
          <SessionRow
            key={s.id}
            session={s}
            onRevoke={
              // The session you are using is never revocable from here -
              // that is what signing out is for.
              s.currentSession
                ? undefined
                : userId
                  ? () => revoke.mutate({ userId, sessionId: s.id })
                  : () => revokeMine.mutate(s.id)
            }
          />
        ))}
      </ul>
    </div>
  );
}

function SelfRevokeAllButton() {
  // The self path uses its own hook: POST /auth/sessions/revoke-all, which is
  // account self-management and carries no permission gate, unlike the admin
  // bulk action above. This was a `return null` stub until 2026-09-11, which
  // is why a user could never revoke their own sessions.
  const { t } = useTranslation('security');
  const revokeAllMine = useRevokeAllMySessions();
  return (
    <Button variant="outline" size="sm" onClick={() => revokeAllMine.mutate()} disabled={revokeAllMine.isPending}>
      {t('sessions.revokeAll')}
    </Button>
  );
}

type SessionRowProps = {
  session: SessionView;
  onRevoke?: () => void;
};

function SessionRow({ session, onRevoke }: SessionRowProps) {
  const { t } = useTranslation('security');
  const isRevoked = !!session.revokedAt;
  return (
    <li
      className={`flex items-center justify-between rounded-md border border-border px-3 py-2 ${
        isRevoked ? 'opacity-60' : ''
      }`}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-mono text-xs text-foreground">{session.handle}</span>
          {session.currentSession && <Badge variant="success">{t('sessions.thisDevice')}</Badge>}
          {isRevoked && <Badge variant="secondary">{t('sessions.revoked')}</Badge>}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>{session.ipAddress || t('sessions.unknownIp')}</span>
          <span className="truncate">{session.userAgent || t('sessions.unknownAgent')}</span>
          <span>
            {t('sessions.created')} {timeAgo(session.createdAt)}
          </span>
        </div>
      </div>
      {onRevoke && (
        <Button
          permission={PERM.SESSIONS_MANAGE}
          variant="ghost"
          size="sm"
          onClick={onRevoke}
          aria-label={t('sessions.revoke')}
        >
          <IconTrash className="size-3.5" />
        </Button>
      )}
    </li>
  );
}
