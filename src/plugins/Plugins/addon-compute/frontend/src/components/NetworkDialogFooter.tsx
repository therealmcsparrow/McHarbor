// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { DialogFooter } from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { COMPUTE_PERM } from '../permissions';

interface NetworkDialogFooterProps {
  editing: boolean;
  canSubmit: boolean;
  pending: boolean;
  onClose: () => void;
  onSubmit: () => void;
}

export function NetworkDialogFooter({ editing, canSubmit, pending, onClose, onSubmit }: NetworkDialogFooterProps) {
  const { t } = useTranslation('compute');

  return (
    <DialogFooter>
      <Button variant="outline" onClick={onClose}>
        {t('actions.cancel')}
      </Button>
      <Button permission={COMPUTE_PERM.MANAGE} onClick={onSubmit} disabled={!canSubmit || pending}>
        {pending && <Spinner className="mr-2 h-4 w-4" />}
        {editing ? t('networks.edit.submit') : t('networks.create.submit')}
      </Button>
    </DialogFooter>
  );
}
