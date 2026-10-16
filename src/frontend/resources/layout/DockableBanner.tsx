// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// DockableBanner — a banner that collapses instead of disappearing.
//
// Transient banners (update available, degraded connection, load error)
// each claimed a full-width band above the data. Three of them at once, on
// a short viewport, pushed row one below the fold.
//
// Dismissing now *docks* the banner rather than deleting it: it collapses
// to a chip in the toolbar and can be reopened from there. Ground rule 3 —
// no capability is removed, only the amount of vertical space it insists on.

import { PlainButton } from '@resources/components/ui/PlainButton';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { useDockedBanners } from '@resources/stores/docked-banners';

const TONE_CLASSES: Record<'info' | 'warning' | 'danger', { wrap: string; icon: string }> = {
  info: { wrap: 'border-info/30 bg-info/10', icon: 'text-info' },
  warning: { wrap: 'border-warning/30 bg-warning/10', icon: 'text-warning' },
  danger: { wrap: 'border-danger/30 bg-danger/10', icon: 'text-danger' },
};

type DockableBannerProps = {
  /** Stable identity. Persisted, so keep it stable across releases. */
  id: string;
  /** Chip label once docked. Must already be translated. */
  label: string;
  tone?: 'info' | 'warning' | 'danger';
  icon?: ReactNode;
  /**
   * A persistent condition (a still-degraded connection) should keep its
   * chip visible even while docked; a transient one (an update notice the
   * user dismissed) should not nag. Defaults to persistent.
   */
  persistent?: boolean;
  /**
   * A condition the operator cannot dismiss their way out of (an
   * unlicensed install) sets this false: no dock control is rendered and a
   * previously docked state is ignored, so the band stays on screen.
   */
  dockable?: boolean;
  /** Called after the banner is docked, for callers tracking their own state. */
  onDock?: () => void;
  /**
   * The child already draws its own bordered box (ErrorState, DegradedStateBanner).
   * Skips this component's chrome so the two don't nest, and floats the dock
   * control over the child's top-right corner instead.
   */
  bare?: boolean;
  children: ReactNode;
  className?: string;
};

export function DockableBanner({
  id,
  label,
  tone = 'info',
  icon,
  persistent = true,
  dockable = true,
  onDock,
  bare = false,
  children,
  className,
}: DockableBannerProps) {
  const { t } = useTranslation('common');
  const docked = useDockedBanners((s) => id in s.docked);
  const dock = useDockedBanners((s) => s.dock);

  if (docked && dockable) return null;

  const toneClass = TONE_CLASSES[tone];
  const dockLabel = t('banner.dock');

  const dockButton = !dockable ? null : (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={dockLabel}
      title={dockLabel}
      onClick={() => {
        dock({ id, label, tone: persistent ? tone : 'info' });
        onDock?.();
      }}
    >
      <IconChevronDown className="size-4" />
    </Button>
  );

  if (bare) {
    return (
      <div className={cn('relative flex-none', className)}>
        {children}
        {dockButton && <div className="absolute top-1.5 right-1.5">{dockButton}</div>}
      </div>
    );
  }

  return (
    <output
      className={cn('flex flex-none items-center gap-3 rounded-lg border px-3 py-2 text-sm', toneClass.wrap, className)}
    >
      {icon && <span className={cn('shrink-0', toneClass.icon)}>{icon}</span>}
      <div className="min-w-0 flex-1">{children}</div>
      {dockButton}
    </output>
  );
}

/**
 * The chips docked banners collapse into. Rendered by `PageToolbar`, so a
 * dismissed banner is always one click from coming back.
 */
export function DockedBannerChips({ className }: { className?: string }) {
  const { t } = useTranslation('common');
  const docked = useDockedBanners((s) => s.docked);
  const restore = useDockedBanners((s) => s.restore);
  const items = Object.values(docked).sort((a, b) => a.dockedAt - b.dockedAt);

  if (items.length === 0) return null;

  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      {items.map((banner) => (
        <PlainButton
          key={banner.id}
          type="button"
          onClick={() => restore(banner.id)}
          title={t('banner.restore')}
          className={cn(
            'inline-flex items-center gap-1.5 border px-2 py-0.5',
            'text-[11px] font-medium transition-colors',
            banner.tone === 'danger' && 'border-danger/40 bg-danger/10 text-danger hover:bg-danger/20',
            banner.tone === 'warning' && 'border-warning/40 bg-warning/10 text-warning hover:bg-warning/20',
            banner.tone === 'info' && 'border-info/40 bg-info/10 text-info hover:bg-info/20',
          )}
        >
          <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
          {banner.label}
          <IconX className="size-3 opacity-60" aria-hidden="true" />
        </PlainButton>
      ))}
    </div>
  );
}
