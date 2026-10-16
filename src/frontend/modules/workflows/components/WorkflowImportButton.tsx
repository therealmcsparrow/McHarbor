// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconUpload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import type { useImportWorkflow } from '../hooks/useWorkflows';

type WorkflowImportButtonProps = {
  importWorkflow: ReturnType<typeof useImportWorkflow>;
};

export function WorkflowImportButton({ importWorkflow }: WorkflowImportButtonProps) {
  const { t } = useTranslation('common');
  const importInputRef = useRef<HTMLInputElement>(null);

  return (
    <>
      <input
        ref={importInputRef}
        type="file"
        accept="application/json,.json,.mcharbor-workflow.json"
        className="hidden"
        onChange={(changeEvent) => {
          const file = changeEvent.target.files?.[0];
          if (file) importWorkflow.mutate(file);
          changeEvent.target.value = '';
        }}
      />
      <Button
        permission={PERM.WORKFLOWS_MANAGE}
        variant="outline"
        onClick={() => importInputRef.current?.click()}
        disabled={importWorkflow.isPending}
      >
        <IconUpload className="size-4" /> {t('workflows.importWorkflow')}
      </Button>
    </>
  );
}
