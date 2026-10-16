// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconKeyboard } from '@tabler/icons-react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from './ui/Dialog';
import { Button } from './ui/Button';
import { useShortcuts } from '../hooks/useShortcuts';
import { getDefaultShortcuts, type ShortcutDefinition } from '../hooks/useShortcuts';

function formatKey(key: string): string {
  return key
    .split('+')
    .map((part) => {
      if (part === 'mod') return 'Ctrl';
      if (part === 'shift') return 'Shift';
      if (part === 'alt') return 'Alt';
      if (part === 'space') return 'Space';
      if (part === 'esc') return 'Esc';
      return part.toUpperCase();
    })
    .join(' + ');
}

type ShortcutsCheatsheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ShortcutsCheatsheet({ open: isOpen, onOpenChange }: ShortcutsCheatsheetProps) {
  const { t } = useTranslation('common');
  const shortcuts = getDefaultShortcuts();

  useShortcuts({
    '?': () => onOpenChange(!isOpen),
  });

  const groups: Record<string, ShortcutDefinition[]> = {};
  for (const sc of shortcuts) {
    const bucket = groups[sc.group] ?? (groups[sc.group] = []);
    bucket.push(sc);
  }

  const groupLabels: Record<string, string> = {
    navigation: t('shortcut.group.navigation'),
    actions: t('shortcut.group.actions'),
    view: t('shortcut.group.view'),
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <IconKeyboard className="size-5 text-primary" />
            <DialogTitle>{t('shortcut.cheatsheetTitle')}</DialogTitle>
          </div>
          <DialogDescription>{t('shortcut.cheatsheetDescription')}</DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-2">
          {Object.entries(groups).map(([group, items]) => (
            <div key={group}>
              <h3 className="mb-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
                {groupLabels[group] ?? group}
              </h3>
              <ul className="space-y-1">
                {items.map((sc) => (
                  <li
                    key={sc.key}
                    className="mc-glass flex items-center justify-between rounded-md px-3 py-1.5 text-sm"
                  >
                    <span>{sc.description}</span>
                    <kbd className="rounded border border-border bg-muted px-2 py-0.5 text-xs">{formatKey(sc.key)}</kbd>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
