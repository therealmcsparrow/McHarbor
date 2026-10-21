// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import { type NarrationSettings, useNarrationSettings, useSaveNarrationSettings } from '../hooks/useNarrationSettings';

function Form({ settings, onDone }: { settings: NarrationSettings; onDone: () => void }) {
  const { t } = useTranslation('incidents');
  const save = useSaveNarrationSettings();
  const [optIn, setOptIn] = useState(settings.narrateOptIn);
  const [proactive, setProactive] = useState(settings.proactive);
  const [cap, setCap] = useState(settings.hourlyCap);
  const [understood, setUnderstood] = useState(settings.narrateOptIn);
  const m = settings.model;
  const needsAck = optIn && !settings.narrateOptIn;

  return (
    <>
      <div className="space-y-4">
        {!settings.modelConfigured || !m ? (
          <Alert variant="warning">{t('settings.noModel')}</Alert>
        ) : (
          <Alert variant={m.local ? 'info' : 'warning'}>
            {t(m.local ? 'settings.destinationLocal' : 'settings.destinationRemote', {
              connection: m.connection,
              provider: m.provider,
              model: m.model,
              url: m.baseUrl ?? '',
            })}
          </Alert>
        )}
        <p className="text-sm text-muted-foreground">{t('settings.whatIsSent')}</p>
        <div className="flex items-center gap-3">
          <Switch id="narrate-opt-in" checked={optIn} onCheckedChange={setOptIn} />
          <Label htmlFor="narrate-opt-in">{t('settings.optIn')}</Label>
        </div>
        {needsAck && (
          <div className="flex items-start gap-2">
            <Checkbox id="narrate-ack" checked={understood} onCheckedChange={(v) => setUnderstood(v === true)} />
            <Label htmlFor="narrate-ack" className="text-sm">
              {t('settings.acknowledge')}
            </Label>
          </div>
        )}
        <div className="flex items-center gap-3">
          <Switch
            id="narrate-proactive"
            checked={proactive && optIn}
            disabled={!optIn}
            onCheckedChange={setProactive}
          />
          <Label htmlFor="narrate-proactive">{t('settings.proactive')}</Label>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="narrate-cap">{t('settings.hourlyCap')}</Label>
          <NumberInput id="narrate-cap" className="w-32" min={1} max={100} value={cap} onChange={setCap} />
          <p className="text-xs text-muted-foreground">{t('settings.hourlyCapHint')}</p>
        </div>
        {save.error && <Alert variant="destructive">{save.error.message}</Alert>}
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onDone}>
          {t('actions.cancel', { ns: 'common' })}
        </Button>
        <Button
          permission={PERM.INCIDENTS_MANAGE}
          disabled={save.isPending || (needsAck && !understood)}
          onClick={() =>
            !save.isPending &&
            save.mutate(
              { envId: settings.envId, narrateOptIn: optIn, proactive: optIn && proactive, hourlyCap: cap },
              { onSuccess: onDone },
            )
          }
        >
          {t('settings.save')}
        </Button>
      </DialogFooter>
    </>
  );
}

export function NarrationSettingsDialog({
  envId,
  onOpenChange,
}: {
  envId: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('incidents');
  const settings = useNarrationSettings(envId);
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('settings.title')}</DialogTitle>
          <DialogDescription>{t('settings.description')}</DialogDescription>
        </DialogHeader>
        {settings.data ? (
          <Form settings={settings.data} onDone={() => onOpenChange(false)} />
        ) : settings.isError ? (
          <ErrorState error={settings.error} onRetry={() => void settings.refetch()} />
        ) : (
          <Spinner />
        )}
      </DialogContent>
    </Dialog>
  );
}
