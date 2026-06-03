// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconArrowsExchange } from '@tabler/icons-react';
import { Trans, useTranslation } from 'react-i18next';
import {
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';

type MoveContainerDialogHeaderProps = {
  containerName: string;
};

export function MoveContainerDialogHeader({ containerName }: MoveContainerDialogHeaderProps) {
  const { t } = useTranslation('containers');

  return (
    <DialogHeader>
      <DialogTitle className="flex items-center gap-2">
        <IconArrowsExchange className="size-4 text-primary" />
        {t('moveDialog.title')}
      </DialogTitle>
      <DialogDescription>
        <Trans
          i18nKey="moveDialog.description"
          ns="containers"
          values={{ name: containerName }}
          components={{ bold: <span className="font-medium text-foreground" /> }}
        />
      </DialogDescription>
    </DialogHeader>
  );
}
