// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { IconArrowUpRight, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { useSelfUpdateState, useDismissUpdate } from '@modules/settings/hooks/useUpdates';

const DISMISS_KEY = 'mcharbor-update-banner-dismissed';

export function UpdateCheckBanner() {
  const { t } = useTranslation('common');
  const { data } = useSelfUpdateState(60_000);
  const dismiss = useDismissUpdate();
  const [dismissedLocal, setDismissedLocal] = useState<string | null>(() => {
    try {
      return window.localStorage.getItem(DISMISS_KEY);
    } catch {
      return null;
    }
  });

  // Reset the local dismiss when the latest version changes — that
  // way the user can re-dismiss an old banner, but a new release
  // re-surfaces without us having to clean localStorage manually.
  useEffect(() => {
    if (!data?.latestVersion) return;
    if (dismissedLocal && dismissedLocal !== data.latestVersion) {
      try {
        window.localStorage.removeItem(DISMISS_KEY);
      } catch {
        // ignore
      }
      setDismissedLocal(null);
    }
  }, [data?.latestVersion, dismissedLocal]);

  // The server-side "last seen version" is the source of truth for
  // the dismiss; the localStorage copy is just an optimistic mirror
  // so the banner doesn't flicker on initial render.
  const latest = data?.latestVersion;
  // If the user already dismissed this exact version (via the
  // banner or the Settings page), the server would have set
  // lastSeenVersion; we don't have that on the public state
  // payload yet, so we only trust the local mirror. The settings
  // tab calls /dismiss which writes the version server-side.
  const isNew = !!(
    data?.updateAvailable &&
    latest &&
    dismissedLocal !== latest
  );

  if (!isNew || !data) {
    return null;
  }

  function handleDismiss() {
    if (!latest) return;
    dismiss.mutate(latest);
    try {
      window.localStorage.setItem(DISMISS_KEY, latest);
    } catch {
      // ignore
    }
    setDismissedLocal(latest);
  }

  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3 border-b border-primary/30 bg-primary/10 px-4 py-2 text-sm text-foreground"
      role="status"
    >
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/20 text-primary">
          <IconArrowUpRight className="size-4" />
        </span>
        <div className="min-w-0 truncate">
          <p className="truncate font-medium">
            {t('updateCheck.banner.title', { version: latest })}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {t('updateCheck.banner.subtitle', { version: data.currentVersion })}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        {data.releaseUrl && (
          <Button asChild variant="outline" size="sm">
            <a
              href={data.releaseUrl}
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('updateCheck.banner.viewRelease')}
            </a>
          </Button>
        )}
        <Button asChild variant="ghost" size="sm">
          <Link
            to="/settings"
            onClick={(e) => e.stopPropagation()}
          >
            {t('updateCheck.banner.openSettings')}
          </Link>
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={handleDismiss}
          aria-label={t('updateCheck.banner.dismiss')}
        >
          <IconX className="size-4" />
        </Button>
      </div>
    </div>
  );
}