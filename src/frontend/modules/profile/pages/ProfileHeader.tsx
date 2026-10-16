// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ProfileHeader — the profile page's hero, built to the same grammar as
// `ContainerDetailHeader`: a square mark on the left, the subject's name in
// the display face beside it, a single row of chips underneath carrying
// state and identifiers, and quiet quick-jump buttons on the right.
//
// The container page shows a state dot because a container is a thing that
// starts and stops. An account has a state too — is it local or owned by an
// identity provider, is a second factor standing behind it — so the same dot
// carries that instead.

import { useTranslation } from 'react-i18next';
import { IconLock, IconShieldLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';

type ProfileHeaderProps = {
  displayName: string;
  username: string;
  email: string | null;
  /** True for identity-provider accounts (OIDC / SAML). */
  identityProviderLocked: boolean;
  /** True for the built-in `system` account, which cannot be edited. */
  systemAccount: boolean;
  /** Undefined while the MFA status is still loading. */
  mfaEnabled?: boolean;
};

export function ProfileHeader({
  displayName,
  username,
  email,
  identityProviderLocked,
  systemAccount,
  mfaEnabled,
}: ProfileHeaderProps) {
  const { t } = useTranslation('common');
  const initials = displayName.charAt(0).toUpperCase();

  return (
    <div className="flex flex-1 flex-col gap-2.5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          {/* The container hero shows the image's own logo here. An account
              has no logo, so it gets the same tile with the initial in it —
              same size, same radius, same ring, so the two pages line up. */}
          <span
            aria-hidden="true"
            className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 text-lg font-semibold text-primary ring-1 ring-primary/25 ring-inset"
          >
            {initials}
          </span>
          <div className="min-w-0">
            <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
              {displayName}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              <StatusIndicator
                tone={systemAccount ? 'neutral' : 'success'}
                label={
                  systemAccount
                    ? t('profile.badges.system')
                    : identityProviderLocked
                      ? t('profile.badges.idp')
                      : t('profile.badges.local')
                }
                size="sm"
              />
              {identityProviderLocked && (
                <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                  <IconLock className="size-3" />
                  {t('actions.locked')}
                </Badge>
              )}
              {mfaEnabled !== undefined && (
                <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                  <IconShieldLock className="size-3" />
                  {mfaEnabled ? t('profile.badges.mfaOn') : t('profile.badges.mfaOff')}
                </Badge>
              )}
              {/* Username and email as mono chips, on the same line as the
                  state — identifiers, not prose. */}
              <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                {username}
              </span>
              {email && (
                <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                  {email}
                </span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
