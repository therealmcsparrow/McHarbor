// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { Button } from '@resources/components/ui/Button';
import { DialogFooter } from '@resources/components/ui/Dialog';

type MoveContainerDialogFooterProps = {
  disabled: boolean;
  onCancel: () => void;
  onMove: () => void;
  t: TFunction<'containers'>;
};

export function MoveContainerDialogFooter({
  disabled,
  onCancel,
  onMove,
  t,
}: MoveContainerDialogFooterProps) {
  return (
    <DialogFooter>
      <Button variant="outline" onClick={onCancel}>
        {t('actions.cancel', { ns: 'common' })}
      </Button>
      <Button onClick={onMove} disabled={disabled}>
        {t('moveDialog.move')}
      </Button>
    </DialogFooter>
  );
}
