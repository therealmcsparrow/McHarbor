// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The image vulnerability scan tab.
//
// This panel used to sit at the bottom of SecurityTab, below the
// capability and privilege fields. It never belonged there: everything
// else on that tab is HostConfig an operator *sets*, while this reports
// what is known to be wrong with the image they chose. Same group —
// "is it safe to run?" — different question, so it gets its own
// destination rather than a scroll past six edit forms.

import { useTranslation } from 'react-i18next';
import { IconPhoto } from '@tabler/icons-react';
import type { ContainerInspect } from '@core/types/docker';
import { ScanSection } from './ScanSection';

type ScansTabProps = {
  container: ContainerInspect;
};

export function ScansTab({ container }: ScansTabProps) {
  const { t } = useTranslation('containers');
  const imageRef = container.Config?.Image ?? '';

  // A container with no resolvable image reference cannot be scanned —
  // there is nothing to name to the scanner. Rare, but it happens with
  // containers created from a bare digest whose image was since pruned,
  // and an empty panel with no explanation is worse than saying so.
  if (!imageRef) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
        <IconPhoto className="size-10 text-muted-foreground" />
        <p className="mt-3 text-sm font-medium text-foreground">{t('scan.noImageTitle')}</p>
        <p className="mt-1 max-w-md text-sm text-muted-foreground">{t('scan.noImageDescription')}</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6">
      <ScanSection imageRef={imageRef} />
    </div>
  );
}
