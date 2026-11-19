// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerProcesses } from '@core/types/docker';

type WireProcesses = {
  Titles?: string[] | null;
  Processes?: (string[] | null)[] | null;
};

/** Docker's top sends `null` for an empty process list, and older agents still pass that through. */
export function normalizeProcesses(raw: WireProcesses | null | undefined): ContainerProcesses {
  return {
    Titles: raw?.Titles ?? [],
    Processes: (raw?.Processes ?? []).map((row) => row ?? []),
  };
}
