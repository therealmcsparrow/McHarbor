// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export function envQuery(envId?: string | null) {
  return envId ? `?env=${envId}` : '';
}

export function restoreSecretCacheKey(secretKey?: string) {
  const value = secretKey?.trim();
  if (!value) return 'current';
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = ((hash << 5) - hash + value.charCodeAt(index)) | 0;
  }
  return `secret:${value.length}:${hash}`;
}

export function containerBackupDownloadUrl(runId: string) {
  return `/api/container-backups/runs/${encodeURIComponent(runId)}/download`;
}
