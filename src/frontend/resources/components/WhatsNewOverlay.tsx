// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSparkles, IconX } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from './ui/Dialog';
import { Button } from './ui/Button';
import { useOnboardingStore } from '@resources/stores/onboarding';

const RELEASE_VERSION_KEY = 'mcharbor-last-seen-version';

type WhatsNewItem = {
  emoji?: string;
  titleKey: string;
  descriptionKey: string;
};

function getReleaseItems(version: string): WhatsNewItem[] {
  if (version === '2.0.7') {
    return [
      { titleKey: 'whatsNew.2_0_7.firstRun.title', descriptionKey: 'whatsNew.2_0_7.firstRun.description' },
      { titleKey: 'whatsNew.2_0_7.help.title', descriptionKey: 'whatsNew.2_0_7.help.description' },
      { titleKey: 'whatsNew.2_0_7.shortcuts.title', descriptionKey: 'whatsNew.2_0_7.shortcuts.description' },
      { titleKey: 'whatsNew.2_0_7.skeletons.title', descriptionKey: 'whatsNew.2_0_7.skeletons.description' },
      { titleKey: 'whatsNew.2_0_7.bulk.title', descriptionKey: 'whatsNew.2_0_7.bulk.description' },
    ];
  }
  return [];
}

export function WhatsNewOverlay({ version }: { version: string }) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);
  const { hasSeenTour, setHasSeenTour } = useOnboardingStore();

  useEffect(() => {
    if (hasSeenTour) return;
    if (typeof window === 'undefined') return;
    try {
      const last = window.localStorage.getItem(RELEASE_VERSION_KEY);
      if (last === version) return;
      const items = getReleaseItems(version);
      if (items.length === 0) return;
      const tmr = window.setTimeout(() => setOpen(true), 1200);
      return () => window.clearTimeout(tmr);
    } catch {
      // localStorage might be disabled — silently skip.
    }
  }, [version, hasSeenTour]);

  const items = getReleaseItems(version);

  const dismiss = () => {
    setOpen(false);
    setHasSeenTour(true);
    try {
      window.localStorage.setItem(RELEASE_VERSION_KEY, version);
    } catch {
      // ignore
    }
  };

  if (items.length === 0) return null;

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : dismiss())}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <IconSparkles className="size-5 text-primary" />
            <DialogTitle>
              {t('whatsNew.title', {
                defaultValue: `What's new in ${version}`,
                version,
              })}
            </DialogTitle>
          </div>
          <DialogDescription>
            {t('whatsNew.description', { defaultValue: 'A quick tour of the latest changes.' })}
          </DialogDescription>
        </DialogHeader>
        <ul className="space-y-3">
          {items.map((item, i) => (
            <li key={i} className="rounded-md border border-border bg-card p-3 text-sm">
              <p className="font-medium text-foreground">
                {item.emoji && <span className="mr-1.5">{item.emoji}</span>}
                {t(item.titleKey)}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t(item.descriptionKey)}
              </p>
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button variant="ghost" onClick={dismiss}>
            <IconX className="mr-1 size-4" />
            {t('whatsNew.dismiss', { defaultValue: 'Dismiss' })}
          </Button>
          <Button onClick={dismiss}>
            {t('whatsNew.gotIt', { defaultValue: 'Got it' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
