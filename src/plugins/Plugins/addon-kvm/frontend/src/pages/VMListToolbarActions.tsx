// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { VM_PERM } from '../permissions';

export function VMListToolbarActions({
  templates,
  onImport,
  onCreate,
}: {
  templates: boolean;
  onImport: () => void;
  onCreate: () => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <>
      {templates ? null : (
        <Button size="sm" variant="outline" permission={VM_PERM.IMPORT} onClick={onImport}>
          <IconDownload className="h-4 w-4" /> {t('actions.import')}
        </Button>
      )}
      <Button size="sm" permission={VM_PERM.MANAGE} onClick={onCreate}>
        <IconPlus className="h-4 w-4" /> {t('actions.create')}
      </Button>
    </>
  );
}
