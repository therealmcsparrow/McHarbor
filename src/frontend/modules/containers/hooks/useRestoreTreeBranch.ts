// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useContainerBackupRestoreFilesystemEntries,
  useContainerBackupRestoreMountEntries,
  useContainerBackupRestoreConfigEntries,
  type ContainerBackupRestoreOption,
} from '@resources/hooks/useContainerBackups';
import { FILESYSTEM_PREFIX, type RestoreFileTreeKind } from '../components/restore-tree-keys';

type BranchArgs = {
  runId: string;
  mountTarget: string;
  innerPath: string;
  secretKey: string;
  kind: RestoreFileTreeKind;
  onError?: (message: string) => void;
};

// useRestoreTreeBranch loads one level of the restore tree and keeps the
// per-level cache the selection helpers walk. Exactly one of the three
// listing endpoints is enabled for a given `kind`, so only that one
// fires a request.
export function useRestoreTreeBranch({ runId, mountTarget, innerPath, secretKey, kind, onError }: BranchArgs) {
  const { t } = useTranslation('containers');
  const [cache, setCache] = useState<Map<string, ContainerBackupRestoreOption[]>>(() => new Map());

  // The mount endpoint requires a non-empty mountTarget; the
  // filesystem endpoint uses the innerPath only and uses
  // FILESYSTEM_PREFIX as the wire label.
  const effectiveMountTarget = kind === 'filesystem' ? FILESYSTEM_PREFIX : mountTarget;

  // Both hooks are always called; the unused one is disabled via
  // the `enabled` flag so no network request fires.
  const filesystemQuery = useContainerBackupRestoreFilesystemEntries(runId, innerPath, secretKey, {
    enabled: kind === 'filesystem' && !!runId,
  });
  const mountQuery = useContainerBackupRestoreMountEntries(runId, effectiveMountTarget, innerPath, secretKey, {
    enabled: kind === 'mount' && !!runId && !!effectiveMountTarget,
  });
  const configQuery = useContainerBackupRestoreConfigEntries(
    runId,
    innerPath,
    secretKey,
    // The config endpoint serves the synthetic root children when
    // parentKey is empty; mirror the filesystem hook's enable rule
    // so the root call always fires. The mount hook keeps the
    // effectiveMountTarget guard because mount rows with an empty
    // target would 404.
    { enabled: kind === 'config' && !!runId },
  );
  const query = kind === 'filesystem' ? filesystemQuery : kind === 'config' ? configQuery : mountQuery;

  // Populate cache with the current level's items as they arrive.
  const [cachedFrom, setCachedFrom] = useState<{ data: unknown; path: string }>({ data: undefined, path: innerPath });
  if (query.data && (cachedFrom.data !== query.data || cachedFrom.path !== innerPath)) {
    const { items: levelItems } = query.data;
    setCachedFrom({ data: query.data, path: innerPath });
    setCache((prev) => {
      const next = new Map(prev);
      next.set(innerPath, levelItems);
      return next;
    });
  }

  // Surface fetch errors to the parent so the dialog can show a
  // toast. The local row also shows a small inline warning.
  useEffect(() => {
    if (query.error && onError) {
      onError(t('backups.restoreFileTreeLoadError'));
    }
  }, [query.error, onError, t]);

  const items = cache.get(innerPath) ?? query.data?.items ?? [];

  return { query, items, cache, effectiveMountTarget };
}
