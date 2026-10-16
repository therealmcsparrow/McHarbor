// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface BackupKeyStatus {
  configured: boolean;
  readable: boolean;
  keyId?: string;
  path: string;
}

/**
 * Whether this McHarbor holds a backup encryption key.
 *
 * Backups are encrypted without exception, so a missing key is not a warning
 * that can wait until the first run fails — it is the difference between a
 * schedule that works and one that fails silently every night.
 *
 * A caller without `settings.view` gets a refusal rather than an answer. That
 * is treated as "cannot tell" rather than "missing": the notice this drives
 * would otherwise appear for every operator who simply cannot read settings.
 */
export function useBackupKeyStatus() {
  return useQuery({
    queryKey: ['backup-key-status'],
    staleTime: 60_000,
    retry: false,
    queryFn: async () => assertSuccess(await api.get<BackupKeyStatus>('/settings/backup-key/status')),
  });
}
