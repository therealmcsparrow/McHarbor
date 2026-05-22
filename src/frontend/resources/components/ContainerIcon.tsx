// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { IconBox } from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { IMAGE_ICON_MAP, IMAGE_LOGO_MAP } from './container-icon-maps';

const LOGO_CDN = 'https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons@master/svg';

const failedLogos = new Set<string>();

function extractBaseName(image: string): string {
  const withoutTag = image.split(':')[0] ?? image;
  const segments = withoutTag.split('/');
  return (segments[segments.length - 1] ?? image).toLowerCase();
}

function getLogoSlug(image: string): string | null {
  const base = extractBaseName(image);
  return IMAGE_LOGO_MAP[base] ?? null;
}

function getIconForImage(image: string): TablerIcon {
  const withoutTag = image.split(':')[0] ?? image;
  const segments = withoutTag.split('/');
  const base = segments[segments.length - 1] ?? image;
  for (const [pattern, icon] of IMAGE_ICON_MAP) {
    if (pattern.test(base)) return icon;
  }
  return IconBox;
}

type ContainerIconProps = {
  image: string;
  className?: string;
};

export function ContainerIcon({ image, className }: ContainerIconProps) {
  const slug = getLogoSlug(image);
  const [imgError, setImgError] = useState(() => (slug ? failedLogos.has(slug) : true));

  if (slug && !imgError) {
    return (
      <img
        src={`${LOGO_CDN}/${slug}.svg`}
        alt=""
        className={cn('size-4 shrink-0 object-contain', className)}
        onError={() => {
          failedLogos.add(slug);
          setImgError(true);
        }}
      />
    );
  }

  const Icon = getIconForImage(image);
  return <Icon className={cn('size-4 shrink-0 text-muted-foreground', className)} />;
}
