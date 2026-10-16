// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconSparkles, IconX } from '@tabler/icons-react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from './ui/Dialog';
import { Button } from './ui/Button';
import { api } from '@core/api/client';
import { getReleaseItems } from './whatsNewItems';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { useWhatsNewStore } from '@resources/stores/whats-new';

const RELEASE_VERSION_KEY = 'mcharbor-last-seen-version';

function readLastSeen(): string | null {
  try {
    return window.localStorage.getItem(RELEASE_VERSION_KEY);
  } catch {
    // localStorage might be disabled — treat it as "nothing recorded".
    return null;
  }
}

function writeLastSeen(version: string): void {
  try {
    window.localStorage.setItem(RELEASE_VERSION_KEY, version);
  } catch {
    // Blocked storage only means the overlay may show again on the next load.
  }
}

/**
 * WhatsNewOverlay announces an *upgrade*, not an arrival.
 *
 * It opens only when this browser previously recorded a different version —
 * i.e. the install was upgraded underneath a user who had already been here.
 * A first login records the running version silently and shows nothing: a
 * brand-new install has no "what changed" to report, and the release notes
 * are noise on top of setting the product up for the first time.
 *
 * It is admin-only. The release highlights are operator-facing (RBAC, audit,
 * backups, plugins) and land on whoever is responsible for the upgrade, not
 * on every user who happens to log in after it.
 *
 * Settings → About can also reopen it on demand, for anyone who can reach
 * that page; only the automatic upgrade popup is limited to admins.
 *
 * The version is read from the running build rather than passed in: a constant
 * somebody has to remember on release day is how this dialog silently stopped
 * opening at all.
 */
export function WhatsNewOverlay() {
  const { t } = useTranslation('common');
  const isOpen = useWhatsNewStore((state) => state.open);
  const show = useWhatsNewStore((state) => state.show);
  const hide = useWhatsNewStore((state) => state.hide);
  const isAdmin = usePermission(PERM.SYSTEM_MANAGE);
  const { data: version } = useQuery({
    queryKey: ['about', 'version'],
    queryFn: () => api.get<{ version: string }>('/about').then((r) => r.data?.version),
    enabled: isAdmin || isOpen,
    staleTime: Infinity,
    retry: false,
  });

  useEffect(() => {
    if (!isAdmin || !version) return;
    if (typeof window === 'undefined') return;

    const last = readLastSeen();
    if (last === version) return;

    // Nothing recorded yet: first login on this install/browser. Remember the
    // running version so the *next* upgrade is what triggers the dialog.
    if (!last) {
      writeLastSeen(version);
      return;
    }

    if (getReleaseItems(version).length === 0) {
      writeLastSeen(version);
      return;
    }

    const tmr = window.setTimeout(show, 1200);
    return () => window.clearTimeout(tmr);
  }, [version, isAdmin, show]);

  const items = version ? getReleaseItems(version) : [];

  const dismiss = () => {
    hide();
    if (version) writeLastSeen(version);
  };

  if (!version || items.length === 0) return null;

  return (
    <Dialog open={isOpen} onOpenChange={(next) => (next ? show() : dismiss())}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <IconSparkles className="size-5 text-primary" />
            <DialogTitle>{t('whatsNew.title', { version })}</DialogTitle>
          </div>
          <DialogDescription>{t('whatsNew.description')}</DialogDescription>
        </DialogHeader>
        <ul className="space-y-3">
          {items.map((item) => (
            <li key={item.titleKey} className="mc-glass rounded-md p-3 text-sm">
              <p className="font-medium text-foreground">
                {item.emoji && <span className="mr-1.5">{item.emoji}</span>}
                {t(item.titleKey)}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">{t(item.descriptionKey)}</p>
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button variant="ghost" onClick={dismiss}>
            <IconX className="mr-1 size-4" />
            {t('whatsNew.dismiss')}
          </Button>
          <Button onClick={dismiss}>{t('whatsNew.gotIt')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
