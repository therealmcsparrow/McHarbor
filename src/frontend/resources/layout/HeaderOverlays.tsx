// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense } from 'react';
import type { HeaderOverlayState } from './useHeaderOverlays';
import { PluginMountBoundary } from '@resources/components/PluginMountBoundary';

const GlobalSearch = lazy(() =>
  import('@resources/components/GlobalSearch').then((module) => ({
    default: module.GlobalSearch,
  })),
);
const ContainerOverview = lazy(() =>
  import('@resources/components/ContainerOverview').then((module) => ({
    default: module.ContainerOverview,
  })),
);
const ImageOverview = lazy(() =>
  import('@resources/components/ImageOverview').then((module) => ({
    default: module.ImageOverview,
  })),
);
const VolumeOverview = lazy(() =>
  import('@resources/components/VolumeOverview').then((module) => ({
    default: module.VolumeOverview,
  })),
);
const StackOverview = lazy(() =>
  import('@resources/components/StackOverview').then((module) => ({
    default: module.StackOverview,
  })),
);

export function HeaderOverlays({ overlays }: { overlays: HeaderOverlayState }) {
  return (
    <>
      {overlays.searchOpen && (
        <PluginMountBoundary>
          <Suspense fallback={null}>
            <GlobalSearch open={overlays.searchOpen} onOpenChange={overlays.setSearchOpen} />
          </Suspense>
        </PluginMountBoundary>
      )}
      {overlays.containerOverviewOpen && (
        <PluginMountBoundary>
          <Suspense fallback={null}>
            <ContainerOverview open={overlays.containerOverviewOpen} onOpenChange={overlays.setContainerOverviewOpen} />
          </Suspense>
        </PluginMountBoundary>
      )}
      {overlays.imageOverviewOpen && (
        <PluginMountBoundary>
          <Suspense fallback={null}>
            <ImageOverview open={overlays.imageOverviewOpen} onOpenChange={overlays.setImageOverviewOpen} />
          </Suspense>
        </PluginMountBoundary>
      )}
      {overlays.volumeOverviewOpen && (
        <PluginMountBoundary>
          <Suspense fallback={null}>
            <VolumeOverview open={overlays.volumeOverviewOpen} onOpenChange={overlays.setVolumeOverviewOpen} />
          </Suspense>
        </PluginMountBoundary>
      )}
      {overlays.stackOverviewOpen && (
        <PluginMountBoundary>
          <Suspense fallback={null}>
            <StackOverview open={overlays.stackOverviewOpen} onOpenChange={overlays.setStackOverviewOpen} />
          </Suspense>
        </PluginMountBoundary>
      )}
    </>
  );
}
