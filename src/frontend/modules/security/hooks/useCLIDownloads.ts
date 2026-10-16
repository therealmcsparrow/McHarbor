// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type CLIArchive = {
  os: 'linux' | 'darwin' | 'windows';
  arch: 'amd64' | 'arm64';
  file: string;
  version: string;
  size: number;
  sha256?: string;
};

export function useCLIDownloads(enabled: boolean) {
  return useQuery({
    queryKey: ['cli', 'downloads'],
    queryFn: () => api.get<CLIArchive[]>('/cli/downloads').then(assertSuccess),
    enabled,
  });
}

export function cliDownloadUrl(archive: Pick<CLIArchive, 'os' | 'arch'>): string {
  return `/api/cli/download/${archive.os}/${archive.arch}`;
}

export function cliLoginCommand(originValue: string, insecure = false): string {
  return insecure ? `mcharbor login --insecure ${originValue}` : `mcharbor login ${originValue}`;
}
