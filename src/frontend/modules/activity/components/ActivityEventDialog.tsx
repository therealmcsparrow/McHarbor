// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { truncateId } from '@resources/utils/format';
import { type ContainerEvent, EventDetails } from './EventDetails';

type ActivityEventDialogProps = {
  selected: ContainerEvent | null;
  onClose: () => void;
  envName: (id: string | null) => string;
};

export function ActivityEventDialog({ selected, onClose, envName }: ActivityEventDialogProps) {
  const { t } = useTranslation('common');
  return (
    <Dialog open={!!selected} onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-2xl p-0">
        <DialogHeader>
          <DialogTitle>{t('activity.detailsTitle')}</DialogTitle>
          <DialogDescription>
            {selected?.containerName ?? truncateId(selected?.containerId)} — {selected?.action}
          </DialogDescription>
        </DialogHeader>
        {selected && (
          <DialogBody className="p-0">
            <EventDetails event={selected} envName={envName} />
          </DialogBody>
        )}
      </DialogContent>
    </Dialog>
  );
}
