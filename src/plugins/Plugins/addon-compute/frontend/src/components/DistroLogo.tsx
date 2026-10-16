// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { IconDisc } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { distroLogo } from '../distroLogo';

interface DistroLogoProps {
  distro: string | undefined;
  size?: 'sm' | 'md';
  className?: string;
}

export function DistroLogo({ distro, size = 'md', className }: DistroLogoProps) {
  const [failed, setFailed] = useState(false);
  const source = distroLogo(distro);
  const box = size === 'sm' ? 'h-5 w-5' : 'h-9 w-9';

  if (!source || failed) {
    return (
      <span
        className={cn(
          box,
          'flex shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground',
          className,
        )}
      >
        <IconDisc className={size === 'sm' ? 'h-3 w-3' : 'h-5 w-5'} aria-hidden="true" />
      </span>
    );
  }

  return (
    <img
      src={source}
      alt=""
      loading="lazy"
      onError={() => setFailed(true)}
      className={cn(box, 'shrink-0 rounded-md object-contain', className)}
    />
  );
}
