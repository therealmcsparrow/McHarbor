// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Link } from 'react-router';
import { IconChevronRight, IconHome, IconStar, IconStarFilled } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { useNavigationStore } from '@resources/stores/navigation';

export type BreadcrumbItem = {
  to?: string;
  label: ReactNode;
  icon?: ReactNode;
};

type PageHeaderProps = {
  title: ReactNode;
  description?: string;
  actions?: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  trackRecent?: { to: string; label: string };
  favoritePath?: string;
  favoriteLabel?: string;
};

function Breadcrumbs({ items }: { items: BreadcrumbItem[] }) {
  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1 text-[11px] text-muted-foreground">
      {items.map((item, i) => {
        const isLast = i === items.length - 1;
        return (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <IconChevronRight className="size-3 opacity-50" />}
            {item.to && !isLast ? (
              <Link
                to={item.to}
                className={cn(
                  'inline-flex items-center gap-1 hover:text-foreground transition-colors',
                )}
              >
                {item.icon}
                {item.label}
              </Link>
            ) : (
              <span className="inline-flex items-center gap-1 text-foreground/80">
                {item.icon}
                {item.label}
              </span>
            )}
          </span>
        );
      })}
    </nav>
  );
}

function FavoriteButton({ to, label }: { to: string; label: string }) {
  const { isFavorite, toggleFavorite } = useNavigationStore();
  const fav = isFavorite(to);
  return (
    <button
      type="button"
      onClick={() => toggleFavorite(to)}
      aria-label={fav ? `Remove ${label} from favorites` : `Add ${label} to favorites`}
      aria-pressed={fav}
      className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
    >
      {fav ? <IconStarFilled className="size-4 text-amber-400" /> : <IconStar className="size-4" />}
    </button>
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
}: PageHeaderProps) {
  const [slotEl, setSlotEl] = useState<HTMLElement | null>(null);
  const { addRecent } = useNavigationStore();

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      setSlotEl(document.getElementById('header-slot'));
    });
    return () => {
      cancelAnimationFrame(frame);
    };
  }, []);

  useEffect(() => {
    if (trackRecent && typeof trackRecent.label === 'string') {
      addRecent(trackRecent.to, trackRecent.label);
    }
  }, [trackRecent?.to, trackRecent?.label, addRecent, trackRecent]);

  if (!slotEl) return null;

  return createPortal(
    <div className="flex flex-1 items-center justify-between gap-3 min-w-0">
      <div className="min-w-0 flex-1">
        {breadcrumbs && breadcrumbs.length > 0 && (
          <div className="mb-0.5">
            <Breadcrumbs
              items={[
                { to: '/dashboard', label: <IconHome className="size-3" /> },
                ...breadcrumbs,
              ]}
            />
          </div>
        )}
        <div className="flex items-center gap-2">
          <h1 className="text-sm font-semibold text-foreground truncate leading-tight">{title}</h1>
          {favoritePath && (
            <FavoriteButton to={favoritePath} label={favoriteLabel ?? String(title)} />
          )}
        </div>
        {description && (
          <p className="text-[11px] text-muted-foreground truncate leading-tight">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-x-2 ml-4">{actions}</div>}
    </div>,
    slotEl,
  );
}
