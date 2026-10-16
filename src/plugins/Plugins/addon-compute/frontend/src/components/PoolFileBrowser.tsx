// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowUp, IconFolder, IconFolderPlus, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { COMPUTE_PERM } from '../permissions';
import { usePoolFiles } from '../hooks/usePoolFiles';
import type { Pool } from '../types';
import { PoolFileDialogs, type DialogState } from './PoolFileDialogs';
import { PoolFileTable } from './PoolFileTable';

/** The path segments of a pool-relative path, for the breadcrumb. */
function segmentsOf(path: string): string[] {
  return path.split('/').filter(Boolean);
}

function parentOf(path: string): string {
  const parts = segmentsOf(path);
  parts.pop();
  return parts.join('/');
}

/**
 * The pool's own filesystem, under its root.
 *
 * Paths are pool-relative throughout — the empty string is the root — because
 * the agent confines every request against the pool's real directory and the
 * absolute path on the host is not something this side should know or show.
 */
export function PoolFileBrowser({ pool }: { pool: Pool }) {
  const { t } = useTranslation('compute');
  const [path, setPath] = useState('');
  const [dialog, setDialog] = useState<DialogState>({ type: 'none' });
  const [draftName, setDraftName] = useState('');
  const [draftBody, setDraftBody] = useState('');

  const listing = usePoolFiles(pool.id, path);

  const closeDialog = () => {
    setDialog({ type: 'none' });
    setDraftName('');
    setDraftBody('');
  };

  if (listing.isError) {
    return <ErrorState onRetry={() => void listing.refetch()} />;
  }

  const entries = listing.data ?? [];

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('pools.files.up')}
          disabled={path === ''}
          onClick={() => setPath(parentOf(path))}
        >
          <IconArrowUp className="size-4" />
        </Button>

        <nav aria-label={t('pools.files.breadcrumb')} className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
          <Button variant="ghost" size="sm" onClick={() => setPath('')}>
            {pool.name}
          </Button>
          {segmentsOf(path).map((segment, index, all) => (
            <span key={all.slice(0, index + 1).join('/')} className="flex items-center gap-1">
              <span aria-hidden="true" className="text-muted-foreground">
                /
              </span>
              <Button variant="ghost" size="sm" onClick={() => setPath(all.slice(0, index + 1).join('/'))}>
                {segment}
              </Button>
            </span>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('pools.files.refresh')}
            onClick={() => void listing.refetch()}
          >
            <IconRefresh className="size-4" />
          </Button>
          <Button
            variant="secondary"
            size="sm"
            permission={COMPUTE_PERM.MANAGE}
            onClick={() => setDialog({ type: 'mkdir' })}
          >
            <IconFolderPlus className="size-4" />
            {t('pools.files.newFolder')}
          </Button>
        </div>
      </div>

      <Card className="min-h-0 flex-1 overflow-auto">
        {listing.isLoading ? (
          <div className="flex items-center justify-center p-10">
            <Spinner />
          </div>
        ) : entries.length === 0 ? (
          <EmptyState
            icon={<IconFolder className="size-6" />}
            title={t('pools.files.empty.title')}
            description={t('pools.files.empty.description')}
          />
        ) : (
          <PoolFileTable
            entries={entries}
            onOpenDir={setPath}
            onEdit={(entry) => {
              setDialog({ type: 'edit', entry });
              setDraftBody('');
            }}
            onRename={(entry) => {
              setDraftName(entry.name);
              setDialog({ type: 'rename', entry });
            }}
            onDelete={(entry) => setDialog({ type: 'delete', entry })}
          />
        )}
      </Card>

      <PoolFileDialogs
        poolId={pool.id}
        path={path}
        dialog={dialog}
        draftName={draftName}
        onDraftNameChange={setDraftName}
        draftBody={draftBody}
        onDraftBodyChange={setDraftBody}
        closeDialog={closeDialog}
      />
    </div>
  );
}
