// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';

type SkipLinkProps = {
  // The id of the main content area. Defaults to "main-content",
  // matching the AppLayout's <main> wrapper.
  targetId?: string;
  className?: string;
};

// SkipLink is the keyboard-only "Skip to main content" link that
// sits at the top of the app shell. It is `sr-only` until it
// receives focus via Tab, at which point it becomes visible at
// the top-left of the viewport so keyboard users can jump past
// the sidebar and header chrome.
//
// The companion to this component is `LiveRegion` /
// `LiveRegionAssertive` in `resources/components/Accessibility.tsx`.
export function SkipLink({ targetId = 'main-content', className }: SkipLinkProps) {
  const { t } = useTranslation('common');
  const label = t('accessibility.skipToMain');
  return (
    <a
      href={`#${targetId}`}
      className={cn(
        'sr-only focus:not-sr-only',
        'fixed top-3 left-3 z-50 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-lg',
        'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
        className,
      )}
    >
      {label}
    </a>
  );
}
