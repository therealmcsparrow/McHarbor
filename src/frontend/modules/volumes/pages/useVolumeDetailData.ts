// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Data derivation for the volume detail page: the containers mounting this
// volume, and the sibling list the breadcrumb offers. Extracted from
// VolumeDetailPage.tsx unchanged.

import { useMemo } from 'react';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import type { ContainerInfo, VolumeInfo } from '@core/types/docker';
import type { VolumeMount } from '../components/VolumeDetailTabContent';

export function useVolumeDetailData(volumeName: string, containers: ContainerInfo[], allVolumes: VolumeInfo[]) {
  // Docker's volume inspect carries no list of the containers using it, so
  // the relationship is read from the other end: every container's `Mounts`
  // entry whose `Name` is this volume. A container that mounts the same
  // volume at two paths appears once per path, which is the truth.
  const mounts: VolumeMount[] = useMemo(() => {
    if (!volumeName) return [];
    const rows: VolumeMount[] = [];
    for (const container of containers) {
      for (const mount of container.Mounts ?? []) {
        if (mount.Type !== 'volume' || mount.Name !== volumeName) continue;
        rows.push({
          containerId: container.Id,
          containerName: container.Names?.[0]?.replace(/^\//, '') ?? container.Id,
          state: container.State,
          destination: mount.Destination,
          readWrite: mount.RW,
        });
      }
    }
    return rows;
  }, [containers, volumeName]);

  const siblings: BreadcrumbSibling[] = useMemo(
    () =>
      allVolumes.map((peer) => ({
        id: peer.Name,
        label: peer.Name,
        to: `/volumes/${encodeURIComponent(peer.Name)}`,
      })),
    [allVolumes],
  );

  return { mounts, siblings };
}
