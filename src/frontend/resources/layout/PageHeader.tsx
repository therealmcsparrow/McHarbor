// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

type PageHeaderProps = {
  title: ReactNode;
  description?: string;
  actions?: ReactNode;
};

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  const [slotEl, setSlotEl] = useState<HTMLElement | null>(null);

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      setSlotEl(document.getElementById('header-slot'));
    });
    return () => {
      cancelAnimationFrame(frame);
    };
  }, []);

  if (!slotEl) return null;

  return createPortal(
    <div className="flex flex-1 items-center justify-between">
      <div className="min-w-0">
        <h1 className="text-sm font-semibold text-foreground truncate leading-tight">{title}</h1>
        {description && (
          <p className="text-[11px] text-muted-foreground truncate leading-tight">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-x-2 ml-4">{actions}</div>}
    </div>,
    slotEl,
  );
}
