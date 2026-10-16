// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { dismissImportJob, isImportRunning, useImportJob } from '../importJob';

interface ImportProgressChipProps {
  onReopen: () => void;
}

export function ImportProgressChip({ onReopen }: ImportProgressChipProps) {
  const { t } = useTranslation('vm');
  const job = useImportJob();

  if (job.phase === 'idle' || job.phase === 'done') return null;

  const running = isImportRunning(job.phase);
  const failed = job.phase === 'failed';

  return (
    <output className="mb-3 flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2">
      {running ? <Spinner className="h-4 w-4 shrink-0" /> : null}

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm text-foreground">
          {t(`import.phase.${job.phase}`)} — {job.fileName}
        </p>
        {job.phase === 'uploading' ? <ProgressBar size="sm" value={job.percent} className="mt-1" /> : null}
      </div>

      {failed ? <StatusError error={job.error} /> : null}

      <Button size="sm" variant="outline" onClick={onReopen}>
        {t('import.reopen')}
      </Button>

      {running ? null : (
        <Button size="icon" variant="ghost" aria-label={t('import.dismiss')} onClick={dismissImportJob}>
          <IconX className="h-4 w-4" />
        </Button>
      )}
    </output>
  );
}
