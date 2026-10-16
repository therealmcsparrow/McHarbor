// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconExternalLink, IconLogin2, IconLogout } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useAIProviderOAuthFlow } from '../hooks/useAIProviderOAuth';

type AIProviderOAuthPanelProps = {
  /** Saved connection to sign in. Null while a connection is still
   *  being created — the token has nowhere to live until it exists. */
  connectionId: string | null;
  /** Permission(s) that gate starting or dropping a sign-in. */
  permission: string | string[];
};

/**
 * The sign-in half of an OAuth connection: start the device flow, show
 * the code the provider expects, and report the result.
 *
 * The code is shown rather than auto-submitted because that is the
 * entire security property of the device grant — the operator reads a
 * short code here and approves it in their own authenticated session on
 * the provider's site, so McHarbor never sees a password.
 */
export function AIProviderOAuthPanel({ connectionId, permission }: AIProviderOAuthPanelProps) {
  const { t } = useTranslation('settings');
  const flow = useAIProviderOAuthFlow(connectionId);

  if (!connectionId) {
    return (
      <div className="rounded-md border border-dashed border-border p-3">
        <p className="text-[11px] text-muted-foreground">{t('aiProviders.auth.signInAfterSave')}</p>
      </div>
    );
  }

  return (
    <div className="space-y-3 rounded-md border border-border bg-background p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">{t('aiProviders.auth.signInTitle')}</p>
          <p className="mt-0.5 truncate text-[11px] text-muted-foreground">
            {flow.connected
              ? t('aiProviders.auth.signedInAs', {
                  account: flow.status?.accountLabel ?? '',
                })
              : t('aiProviders.auth.notSignedIn')}
          </p>
        </div>
        {flow.connected ? (
          <Badge variant="default">
            <IconCheck className="mr-1 size-3" />
            {t('aiProviders.auth.signedIn')}
          </Badge>
        ) : (
          <Badge variant="secondary">{t('aiProviders.auth.signInRequired')}</Badge>
        )}
      </div>

      {flow.pending && flow.status?.userCode && (
        <div className="space-y-2 rounded-md border border-border bg-card p-3">
          <p className="text-xs text-muted-foreground">{t('aiProviders.auth.enterCode')}</p>
          <p className="font-mono text-xl tracking-[0.3em] text-foreground select-all">{flow.status.userCode}</p>
          <div className="flex flex-wrap items-center gap-2">
            {flow.status.verificationUri && (
              <Button variant="outline" size="sm" asChild>
                <a href={flow.status.verificationUri} target="_blank" rel="noreferrer noopener">
                  <IconExternalLink className="mr-1 size-4" />
                  {t('aiProviders.auth.openProviderPage')}
                </a>
              </Button>
            )}
            <span className="flex items-center gap-1 text-[11px] text-muted-foreground">
              <Spinner size="sm" />
              {t('aiProviders.auth.waitingForApproval')}
            </span>
          </div>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button permission={permission} size="sm" onClick={flow.start} disabled={flow.isStarting || flow.pending}>
          <IconLogin2 className="mr-1 size-4" />
          {flow.connected ? t('aiProviders.auth.signInAgain') : t('aiProviders.auth.signIn')}
        </Button>
        {flow.connected && (
          <Button
            permission={permission}
            variant="outline"
            size="sm"
            onClick={flow.disconnect}
            disabled={flow.isDisconnecting}
          >
            <IconLogout className="mr-1 size-4" />
            {t('aiProviders.auth.disconnect')}
          </Button>
        )}
      </div>
    </div>
  );
}
