// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { IconTrash, IconUpload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Select } from '@resources/components/ui/Select';
import { useImports, useDeleteImport } from '../hooks/useImports';
import { adoptImportJob, startImportUpload } from '../importJob';
import type { ImportForm } from '../hooks/useImportForm';
import { formatBytes } from '../format';
import { VM_PERM } from '../permissions';

export function ImportUploadStep({
  environments,
  form,
}: {
  environments: { id: string; name: string }[];
  form: ImportForm;
}) {
  const { t } = useTranslation('vm');
  const fileInput = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();
  const { data: importList } = useImports();
  const removeImport = useDeleteImport();
  const { envId, setEnvId, running, job } = form;

  return (
    <div className="grid gap-3">
      {(importList?.imports ?? []).length > 0 ? (
        <div className="rounded-md border border-border">
          <p className="border-b border-border px-3 py-2 text-sm font-medium text-foreground">
            {t('import.pending.title')}
          </p>
          <ul className="divide-y divide-border">
            {(importList?.imports ?? []).map((record) => (
              <li key={record.id} className="flex items-center gap-3 px-3 py-2">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm text-foreground">{record.fileName}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {record.sourceKind} · {formatBytes(record.sizeBytes)} · {t(`import.state.${record.state}`)}
                  </p>
                </div>
                <Button size="sm" variant="outline" permission={VM_PERM.IMPORT} onClick={() => adoptImportJob(record)}>
                  {t('import.pending.continue')}
                </Button>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  permission={VM_PERM.IMPORT}
                  aria-label={t('import.pending.discard')}
                  title={t('import.pending.discard')}
                  disabled={removeImport.isPending}
                  onClick={() => removeImport.mutate(record.id)}
                >
                  <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="grid gap-1.5">
        <Label htmlFor="import-env">{t('import.field.host')}</Label>
        <Select
          value={envId}
          onChange={setEnvId}
          options={environments.map((env) => ({ value: env.id, label: env.name }))}
          ariaLabel={t('import.field.host')}
        />
        <p className="text-xs text-muted-foreground">{t('import.field.hostHelp')}</p>
      </div>

      <p className="text-sm text-muted-foreground">{t('import.formats')}</p>
      <input
        ref={fileInput}
        type="file"
        className="hidden"
        accept=".ova,.ovf,.vmdk,.vhdx,.vhd,.vdi,.hdd,.hds,.qcow2,.qcow,.img,.raw"
        onChange={(evt) => {
          const file = evt.target.files?.[0];
          if (file && envId) startImportUpload(file, envId, queryClient);
        }}
      />
      <Button permission={VM_PERM.IMPORT} disabled={running || !envId} onClick={() => fileInput.current?.click()}>
        <IconUpload className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {t('import.choose')}
      </Button>
      {job.phase === 'uploading' ? (
        <div className="grid gap-1">
          <ProgressBar value={job.percent} />
          <p className="text-xs text-muted-foreground">{t('import.uploading', { percent: job.percent })}</p>
        </div>
      ) : null}
    </div>
  );
}
