// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useEffect, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { createPortal } from 'react-dom';
import { IconHome, IconStar, IconStarFilled } from '@tabler/icons-react';
import { useHeaderSlot } from '@resources/hooks/useHeaderSlot';
import { useHeaderSlot as useHeaderSlotStore } from '@resources/stores/headerSlot';
import { useNavigationStore } from '@resources/stores/navigation';
import { Breadcrumb, type BreadcrumbCrumb } from './Breadcrumb';

/**
 * Kept as an alias so the existing importers keep compiling. The shape was
 * always identical to the layout breadcrumb's own crumb type; this file used
 * to carry a second, quieter implementation of the same component.
 */
export type BreadcrumbItem = BreadcrumbCrumb;

type PageHeaderProps = {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  trackRecent?: { to: string; label: string };
  favoritePath?: string;
  favoriteLabel?: string;
  headingLeaf?: boolean;
};

function FavoriteButton({ to, label }: { to: string; label: string }) {
  const { t } = useTranslation('common');
  const { isFavorite, toggleFavorite } = useNavigationStore();
  const fav = isFavorite(to);
  return (
    <PlainButton
      type="button"
      onClick={() => toggleFavorite(to)}
      aria-label={fav ? t('favorites.remove', { label }) : t('favorites.add', { label })}
      aria-pressed={fav}
      className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
    >
      {fav ? <IconStarFilled className="size-4 text-warning" /> : <IconStar className="size-4" />}
    </PlainButton>
  );
}

export function PageHeader({
  title,
  description,
  actions,
  breadcrumbs,
  trackRecent,
  favoritePath,
  favoriteLabel,
  headingLeaf,
}: PageHeaderProps) {
  const { t } = useTranslation('common');
  const slotEl = useHeaderSlot();
  const bandEl = useHeaderSlot('pagehead-slot');
  const { addRecent } = useNavigationStore();
  const setHeadActive = useHeaderSlotStore((store) => store.setActive);

  useEffect(() => {
    setHeadActive(true);
    return () => setHeadActive(false);
  }, [setHeadActive]);

  useEffect(() => {
    if (trackRecent && typeof trackRecent.label === 'string') {
      addRecent(trackRecent.to, trackRecent.label);
    }
  }, [trackRecent?.to, trackRecent?.label, addRecent, trackRecent]);

  return (
    <>
      {slotEl &&
        createPortal(
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <Breadcrumb
              crumbs={[
                ...(window.location.pathname === '/dashboard'
                  ? []
                  : [
                      {
                        to: '/dashboard',
                        label: (
                          <>
                            <IconHome className="size-3" aria-hidden="true" />
                            <span className="sr-only">{t('nav.dashboard')}</span>
                          </>
                        ),
                      },
                    ]),
                ...(breadcrumbs ?? []),
                { label: title },
              ]}
              headingLeaf={headingLeaf}
              className="min-w-0 flex-1"
            />
            {favoritePath && (
              <FavoriteButton
                to={favoritePath}
                label={favoriteLabel ?? (typeof title === 'string' || typeof title === 'number' ? String(title) : '')}
              />
            )}
          </div>,
          slotEl,
        )}
      {bandEl &&
        (description || actions) &&
        createPortal(
          <div className="flex h-20 shrink-0 items-center gap-5 overflow-hidden border-b border-border bg-canvas-2 px-6">
            {typeof description === 'string' ? (
              <p className="max-w-[62ch] text-sm text-muted-foreground">{description}</p>
            ) : (
              description
            )}
            {actions && <div className="ml-auto flex shrink-0 items-center gap-x-2">{actions}</div>}
          </div>,
          bandEl,
        )}
    </>
  );
}
