// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { useHostServiceOverride } from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { OverrideEditor } from './HostServiceOverrideEditor';

type HostServiceOverrideDialogProps = {
  host: HostTarget;
  unit: string | null;
  onOpenChange: (open: boolean) => void;
  onRevert: (unit: string) => void;
};

export function HostServiceOverrideDialog({ host, unit, onOpenChange, onRevert }: HostServiceOverrideDialogProps) {
  const { t } = useTranslation('environments');
  const override = useHostServiceOverride(host, unit);

  return (
    <Dialog open={unit !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('hostServices.override.title', { unit: unit ?? '' })}</DialogTitle>
          <DialogDescription>{t('hostServices.override.description')}</DialogDescription>
        </DialogHeader>
        {override.isPending && (
          <DialogBody>
            <div
              className="flex min-h-40 items-center justify-center"
              aria-label={t('common.loading', { ns: 'common' })}
            >
              <Spinner />
            </div>
          </DialogBody>
        )}
        {override.isError && (
          <DialogBody>
            <Alert
              variant="destructive"
              title={t('hostServices.override.loadFailed')}
              description={override.error.message}
            />
          </DialogBody>
        )}
        {override.data && unit && (
          <OverrideEditor
            key={unit}
            host={host}
            unit={unit}
            override={override.data}
            onClose={() => onOpenChange(false)}
            onRevert={() => onRevert(unit)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
