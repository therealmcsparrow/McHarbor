// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { type ReconcilerSettings, useReconcilerSettings, useSetConfirmPasses } from '../hooks/useReconcilerSettings';
import { EnforcementSettings } from './EnforcementSettings';
import { refusalText } from '../hooks/refusal';

function Debounce({ settings }: { settings: ReconcilerSettings }) {
  const { t } = useTranslation('reconciler');
  const save = useSetConfirmPasses();
  const [passes, setPasses] = useState(settings.confirmPasses);
  return (
    <section className="space-y-2">
      <Label htmlFor="confirm-passes">{t('settings.confirmPasses')}</Label>
      <div className="flex items-center gap-2">
        <NumberInput id="confirm-passes" className="w-32" min={1} max={10} value={passes} onChange={setPasses} />
        <Button
          variant="outline"
          permission={PERM.RECONCILER_MANAGE}
          disabled={save.isPending || passes === settings.confirmPasses}
          onClick={() => save.mutate(passes)}
        >
          {t('settings.save')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('settings.confirmPassesHint')}</p>
      {save.error && <Alert variant="destructive">{refusalText(save.error, t)}</Alert>}
    </section>
  );
}

export function ReconcilerSettingsDialog({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation('reconciler');
  const settings = useReconcilerSettings();
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('settings.title')}</DialogTitle>
          <DialogDescription>{t('settings.description')}</DialogDescription>
        </DialogHeader>
        {settings.data ? (
          <div className="space-y-6">
            <Debounce settings={settings.data} />
            <EnforcementSettings
              key={`${settings.data.enforceEnabled}|${settings.data.maxActionsPerPass}|${settings.data.cooldownSeconds}|${settings.data.failureThreshold}`}
              settings={settings.data}
            />
          </div>
        ) : (
          <Spinner />
        )}
      </DialogContent>
    </Dialog>
  );
}
