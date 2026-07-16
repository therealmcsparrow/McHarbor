// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';
import type { DockerSystemInfo } from '@core/types/docker';

export function useDockerInfo(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const uploadActive = useEnvironmentUploadActive(envId);
  return useQuery({
    queryKey: ['docker-info', envId],
    queryFn: () =>
      api
        .get<DockerSystemInfo>('/docker/info', envId ? { env: envId } : {})
        .then((r) => r.data),
    refetchInterval: uploadActive ? false : 60_000,
    enabled: enabled && !uploadActive,
  });
}