// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import {
  filterApplicableShells,
  type ShellInfo,
  type TerminalShellOption,
} from '@resources/components/TerminalShellOptions';

export type { ShellInfo } from '@resources/components/TerminalShellOptions';

export type TerminalUser = {
  id: string;
  label: string;
  user: string;
};

export function useContainerShells(containerId: string | undefined) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-shells', envId, containerId],
    queryFn: () =>
      api
        .post<ShellInfo[]>(`/containers/${containerId}/shells${envId ? `?env=${envId}` : ''}`, {})
        .then((r) => r.data ?? []),
    enabled: !!containerId,
    staleTime: 60_000,
  });
}

// useApplicableShells returns the SHELL_OPTIONS subset that the
// container has installed, plus a `coerceShell` helper that snaps a
// previously-selected value back to the empty "auto-detect" value when
// the probed list drops it. While the probe is in flight (data ===
// undefined) or when the probe returns an empty list, the full set is
// returned — better to over-show than to silently hide the only shell
// the container actually has.
export function useApplicableShells(containerId: string | undefined): {
  options: TerminalShellOption[];
  coerceShell: (value: string) => string;
} {
  const { data } = useContainerShells(containerId);
  const options = filterApplicableShells(data);
  const allowed = new Set(options.map((o) => o.value));
  const coerceShell = (value: string) => (allowed.has(value) ? value : '');
  return { options, coerceShell };
}
