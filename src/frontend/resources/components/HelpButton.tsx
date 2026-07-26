// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHelp, IconBook, IconExternalLink } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from './ui/Dialog';
import { Button } from './ui/Button';
import { cn } from '@resources/utils/cn';

type HelpLink = {
  label: string;
  href: string;
};

type HelpTopic = {
  id?: string;
  titleKey?: string;
  descriptionKey?: string;
  title?: string;
  description?: string;
  links?: HelpLink[];
};

type HelpRegistry = {
  [route: string]: {
    titleKey: string;
    descriptionKey: string;
    links?: HelpLink[];
  };
};

const helpRegistry: HelpRegistry = {
  '/containers': {
    titleKey: 'help.containers.title',
    descriptionKey: 'help.containers.description',
    links: [
      { label: 'Docker container docs', href: 'https://docs.docker.com/engine/containers/' },
    ],
  },
  '/images': {
    titleKey: 'help.images.title',
    descriptionKey: 'help.images.description',
  },
  '/volumes': {
    titleKey: 'help.volumes.title',
    descriptionKey: 'help.volumes.description',
  },
  '/networks': {
    titleKey: 'help.networks.title',
    descriptionKey: 'help.networks.description',
  },
  '/stacks': {
    titleKey: 'help.stacks.title',
    descriptionKey: 'help.stacks.description',
  },
  '/environments': {
    titleKey: 'help.environments.title',
    descriptionKey: 'help.environments.description',
  },
  '/appstore': {
    titleKey: 'help.appstore.title',
    descriptionKey: 'help.appstore.description',
  },
  '/workflows': {
    titleKey: 'help.workflows.title',
    descriptionKey: 'help.workflows.description',
  },
  '/git': {
    titleKey: 'help.git.title',
    descriptionKey: 'help.git.description',
  },
  '/gitops': {
    titleKey: 'help.gitops.title',
    descriptionKey: 'help.gitops.description',
  },
  '/dashboard': {
    titleKey: 'help.dashboard.title',
    descriptionKey: 'help.dashboard.description',
  },
  '/settings': {
    titleKey: 'help.settings.title',
    descriptionKey: 'help.settings.description',
  },
};

export function getHelpForRoute(pathname: string) {
  for (const route of Object.keys(helpRegistry).sort((a, b) => b.length - a.length)) {
    if (pathname.startsWith(route)) {
      return helpRegistry[route];
    }
  }
  return null;
}

type HelpButtonProps = {
  topic?: HelpTopic;
  className?: string;
};

export function HelpButton({ topic, className }: HelpButtonProps) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        onClick={() => setOpen(true)}
        className={className}
        aria-label={t('help.open', { defaultValue: 'Open help' })}
      >
        <IconHelp className="size-4" />
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <div className="flex items-center gap-2">
              <IconBook className="size-5 text-primary" />
              <DialogTitle>
                {topic?.titleKey
                  ? t(topic.titleKey)
                  : topic?.title ?? t('help.title', { defaultValue: 'Help' })}
              </DialogTitle>
            </div>
            <DialogDescription>
              {topic?.descriptionKey
                ? t(topic.descriptionKey)
                : topic?.description ??
                  t('help.description', {
                    defaultValue: 'Find documentation and quick tips.',
                  })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {topic?.links && topic.links.length > 0 && (
              <div>
                <h4 className="mb-2 text-xs font-semibold text-muted-foreground">
                  {t('help.relatedLinks', { defaultValue: 'Related links' })}
                </h4>
                <ul className="space-y-1">
                  {topic.links.map((link) => (
                    <li key={link.href}>
                      <a
                        href={link.href}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
                      >
                        {link.label}
                        <IconExternalLink className="size-3" />
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <div>
              <h4 className="mb-2 text-xs font-semibold text-muted-foreground">
                {t('help.keyboardShortcuts', { defaultValue: 'Keyboard shortcuts' })}
              </h4>
              <ul className="space-y-1 text-sm">
                <li className="flex items-center justify-between">
                  <span>{t('shortcut.commandPalette', { defaultValue: 'Command palette' })}</span>
                  <kbd className="rounded border border-border bg-muted px-2 py-0.5 text-xs">Ctrl+K</kbd>
                </li>
                <li className="flex items-center justify-between">
                  <span>{t('shortcut.globalSearch', { defaultValue: 'Global search' })}</span>
                  <kbd className="rounded border border-border bg-muted px-2 py-0.5 text-xs">Ctrl+/</kbd>
                </li>
                <li className="flex items-center justify-between">
                  <span>{t('shortcut.closeDialog', { defaultValue: 'Close dialog' })}</span>
                  <kbd className="rounded border border-border bg-muted px-2 py-0.5 text-xs">Esc</kbd>
                </li>
              </ul>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t('actions.close', { defaultValue: 'Close' })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

type AutoHelpButtonProps = {
  pathname: string;
  className?: string;
};

export function AutoHelpButton({ pathname, className }: AutoHelpButtonProps) {
  const help = getHelpForRoute(pathname);
  if (!help) return null;
  return (
    <HelpButton
      className={className}
      topic={{
        titleKey: help.titleKey,
        descriptionKey: help.descriptionKey,
        links: help.links,
      }}
    />
  );
}

type HelpHintProps = {
  children: ReactNode;
  className?: string;
};

export function HelpHint({ children, className }: HelpHintProps) {
  return (
    <p
      className={cn(
        'rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground',
        className,
      )}
    >
      {children}
    </p>
  );
}

type ContextualHintProps = {
  messageKey: string;
  defaultValue: string;
  className?: string;
};

export function ContextualHint({
  messageKey,
  defaultValue,
  className,
}: ContextualHintProps) {
  const { t } = useTranslation('common');
  return (
    <HelpHint className={className}>{t(messageKey, { defaultValue })}</HelpHint>
  );
}

