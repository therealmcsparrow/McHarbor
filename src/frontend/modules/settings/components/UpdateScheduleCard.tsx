// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCalendarTime } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Alert } from '@resources/components/ui/Alert';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import {
  useSaveSelfUpdateSettings,
  useSelfUpdateSettings,
  useSelfUpdateState,
  type SelfUpdateSettings,
} from '../hooks/useUpdates';

type Schedule = Pick<SelfUpdateSettings, 'checkCron' | 'installCron' | 'installWindowMinutes'>;

function scheduleOf(s: SelfUpdateSettings): Schedule {
  return { checkCron: s.checkCron, installCron: s.installCron, installWindowMinutes: s.installWindowMinutes };
}

function ScheduleForm({ settings }: { settings: SelfUpdateSettings }) {
  const { t } = useTranslation('settings');
  const { data: state } = useSelfUpdateState();
  const save = useSaveSelfUpdateSettings();
  const [draft, setDraft] = useState<Schedule>(() => scheduleOf(settings));
  const saved = scheduleOf(settings);
  const dirty =
    draft.checkCron !== saved.checkCron ||
    draft.installCron !== saved.installCron ||
    draft.installWindowMinutes !== saved.installWindowMinutes;
  const automatic = settings.autoInstall || settings.autoUpdateAgents;

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <ReadableScheduleField
          value={draft.checkCron}
          label={t('about.updateSchedule.check')}
          onChange={(checkCron) => setDraft((d) => ({ ...d, checkCron }))}
        />
        {state?.nextCheckAt && settings.enabled && (
          <p className="text-xs text-muted-foreground">
            {t('about.updateSchedule.nextCheck', { date: formatDate(state.nextCheckAt) })}
          </p>
        )}
      </div>
      <div className="space-y-1">
        <ReadableScheduleField
          value={draft.installCron}
          label={t('about.updateSchedule.install')}
          hint={t('about.updateSchedule.installHint')}
          onChange={(installCron) => setDraft((d) => ({ ...d, installCron }))}
        />
        {state?.nextInstallAt && automatic && (
          <p className="text-xs text-muted-foreground">
            {t('about.updateSchedule.nextInstall', { date: formatDate(state.nextInstallAt) })}
          </p>
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="upd-window">{t('about.updateSchedule.windowLabel')}</Label>
        <NumberInput
          id="upd-window"
          className="w-32"
          min={15}
          max={720}
          value={draft.installWindowMinutes}
          onChange={(installWindowMinutes) => setDraft((d) => ({ ...d, installWindowMinutes }))}
        />
        <p className="text-xs text-muted-foreground">{t('about.updateSchedule.windowHint')}</p>
      </div>
      {state?.installDeferred && (
        <Alert variant="warning">
          {state.installSkipped
            ? t('about.updateSchedule.skipped', { reason: t(`about.updateSchedule.deferred.${state.installDeferred}`) })
            : t(`about.updateSchedule.deferred.${state.installDeferred}`)}
        </Alert>
      )}
      {save.error && <Alert variant="destructive">{t('about.updateSchedule.badCron')}</Alert>}
      <div className="flex justify-end gap-2">
        <Button variant="outline" disabled={!dirty || save.isPending} onClick={() => setDraft(saved)}>
          {t('common:actions.cancel')}
        </Button>
        <Button
          permission={PERM.UPDATES_MANAGE}
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate({ ...settings, ...draft })}
        >
          {t('about.updateSchedule.save')}
        </Button>
      </div>
    </div>
  );
}

export function UpdateScheduleCard() {
  const { t } = useTranslation('settings');
  const { data: settings } = useSelfUpdateSettings();
  if (!settings) return null;

  return (
    <div className="rounded-lg border border-border p-5">
      <div className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <IconCalendarTime className="size-4" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('about.updateSchedule.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('about.updateSchedule.subtitle')}</p>
        </div>
      </div>
      <ScheduleForm
        key={`${settings.checkCron}|${settings.installCron}|${settings.installWindowMinutes}`}
        settings={settings}
      />
    </div>
  );
}
