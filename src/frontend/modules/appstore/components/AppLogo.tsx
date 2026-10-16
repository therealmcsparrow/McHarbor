// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { cn } from '@resources/utils/cn';

interface AppLogoProps {
  appName: string;
  logo?: string;
  imgClassName?: string;
}

// Catalog logos are third-party URLs that 404 or rate-limit, so a failed load
// falls back to the initial rather than leaving an empty tile.
export function AppLogo({ appName, logo, imgClassName }: AppLogoProps) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const showImage = !!logo && failedSrc !== logo;

  return (
    <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
      {showImage ? (
        <img
          src={logo}
          alt={appName}
          loading="lazy"
          className={cn('object-contain', imgClassName ?? 'size-7')}
          onError={() => setFailedSrc(logo)}
        />
      ) : (
        <span className="text-lg font-bold text-muted-foreground" aria-hidden="true">
          {appName.charAt(0).toUpperCase()}
        </span>
      )}
    </div>
  );
}
