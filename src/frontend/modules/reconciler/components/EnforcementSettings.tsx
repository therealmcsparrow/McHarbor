// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import { formatDate } from '@resources/utils/format';
import {
  type EnforcementInput,
  type ReconcilerSettings,
  useResetBreaker,
  useSetEnforcement,
} from '../hooks/useReconcilerSettings';
import { refusalText } from '../hooks/refusal';

export function EnforcementSettings({ settings }: { settings: ReconcilerSettings }) {
  const { t } = useTranslation('reconciler');
  const gate = usePermissionGate(PERM.RECONCILER_ENFORCE);
  const save = useSetEnforcement();
  const reset = useResetBreaker();
  const [confirmReset, setConfirmReset] = useState(false);
  const [draft, setDraft] = useState<EnforcementInput>({
    enforceEnabled: settings.enforceEnabled,
    maxActionsPerPass: settings.maxActionsPerPass,
    cooldownSeconds: settings.cooldownSeconds,
    failureThreshold: settings.failureThreshold,
  });
  const set = (patch: Partial<EnforcementInput>) => setDraft((d) => ({ ...d, ...patch }));

  return (
    <section className="space-y-3">
      <h4 className="text-sm font-semibold text-foreground">{t('settings.enforcement')}</h4>
      <p className="text-xs text-muted-foreground">{t('settings.enforcementHint')}</p>
      {settings.breakerOpen && (
        <Alert variant="destructive">
          <p>
            {t('settings.breakerOpen', { time: settings.breakerOpenedAt ? formatDate(settings.breakerOpenedAt) : '' })}
          </p>
          {settings.breakerReason && <p className="mt-1 font-mono text-xs break-all">{settings.breakerReason}</p>}
          <div className="mt-2 flex gap-2">
            {confirmReset ? (
              <>
                <Button size="sm" variant="outline" onClick={() => setConfirmReset(false)}>
                  {t('actions.cancel', { ns: 'common' })}
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  permission={PERM.RECONCILER_ENFORCE}
                  disabled={reset.isPending}
                  onClick={() =>
                    !reset.isPending && reset.mutate(undefined, { onSuccess: () => setConfirmReset(false) })
                  }
                >
                  {t('settings.confirmReset')}
                </Button>
              </>
            ) : (
              <Button
                size="sm"
                variant="outline"
                permission={PERM.RECONCILER_ENFORCE}
                onClick={() => setConfirmReset(true)}
              >
                {t('settings.reset')}
              </Button>
            )}
          </div>
          {reset.error && <p className="mt-2 text-xs">{refusalText(reset.error, t)}</p>}
        </Alert>
      )}
      <div className="flex items-center gap-3">
        <Switch
          id="enforce-enabled"
          checked={draft.enforceEnabled}
          disabled={!gate.allowed}
          onCheckedChange={(enforceEnabled) => set({ enforceEnabled })}
        />
        <Label htmlFor="enforce-enabled">{t('settings.enforceEnabled')}</Label>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="enf-actions">{t('settings.maxActions')}</Label>
          <NumberInput
            id="enf-actions"
            min={1}
            max={25}
            value={draft.maxActionsPerPass}
            disabled={!gate.allowed}
            onChange={(v) => set({ maxActionsPerPass: v })}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="enf-cooldown">{t('settings.cooldownMinutes')}</Label>
          <NumberInput
            id="enf-cooldown"
            min={0}
            max={1440}
            value={Math.round(draft.cooldownSeconds / 60)}
            disabled={!gate.allowed}
            onChange={(v) => set({ cooldownSeconds: v * 60 })}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="enf-threshold">{t('settings.threshold')}</Label>
          <NumberInput
            id="enf-threshold"
            min={1}
            max={20}
            value={draft.failureThreshold}
            disabled={!gate.allowed}
            onChange={(v) => set({ failureThreshold: v })}
          />
        </div>
      </div>
      {save.error && <Alert variant="destructive">{refusalText(save.error, t)}</Alert>}
      <div className="flex justify-end">
        <Button
          permission={PERM.RECONCILER_ENFORCE}
          disabled={save.isPending}
          onClick={() => !save.isPending && save.mutate(draft)}
        >
          {t('settings.saveEnforcement')}
        </Button>
      </div>
    </section>
  );
}
