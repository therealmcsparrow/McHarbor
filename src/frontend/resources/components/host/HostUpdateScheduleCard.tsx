// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import {
  hostPermission,
  useSaveHostUpdateSchedule,
  type HostTarget,
  type HostUpdateSchedule,
} from '@resources/hooks/useHostUpdates';

const DAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const;

type HostUpdateScheduleCardProps = {
  target: HostTarget;
  schedule?: HostUpdateSchedule;
  disabled?: boolean;
  recoveryUnsupported?: boolean;
};

export function HostUpdateScheduleCard({
  target,
  schedule,
  disabled = false,
  recoveryUnsupported = false,
}: HostUpdateScheduleCardProps) {
  const { t } = useTranslation('environments');
  const save = useSaveHostUpdateSchedule(target);

  const dayLabels = t('detail.retention.daysShort', {
    returnObjects: true,
  }) as Record<string, string>;

  const [enabled, setEnabled] = useState(false);
  const [time, setTime] = useState('03:00');
  const [days, setDays] = useState<string[]>([...DAYS]);
  const [withNewPackages, setWithNewPackages] = useState(false);
  const [skipUnavailable, setSkipUnavailable] = useState(false);
  const [removeUnused, setRemoveUnused] = useState(false);

  const [syncedSchedule, setSyncedSchedule] = useState<typeof schedule>(undefined);
  if (schedule && schedule !== syncedSchedule) {
    setSyncedSchedule(schedule);
    setEnabled(schedule.enabled);
    setTime(schedule.time || '03:00');
    setDays(schedule.days?.length ? schedule.days : [...DAYS]);
    setWithNewPackages(schedule.withNewPackages ?? false);
    setSkipUnavailable(schedule.skipUnavailable ?? false);
    setRemoveUnused(schedule.removeUnused ?? false);
  }

  const toggleDay = (day: string, on: boolean) => setDays(on ? [...days, day] : days.filter((entry) => entry !== day));

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4">
        <h3 className="text-sm font-medium text-foreground">{t('detail.hostUpdates.scheduleTitle')}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">{t('detail.hostUpdates.scheduleDescription')}</p>
      </div>

      <div className="space-y-4">
        <div className="flex items-start gap-3">
          <Switch id="host-update-schedule" checked={enabled} disabled={disabled} onCheckedChange={setEnabled} />
          <Label htmlFor="host-update-schedule" className="text-sm font-normal">
            {t('detail.hostUpdates.scheduleEnabled')}
          </Label>
        </div>

        <div className="max-w-40">
          <Label className="mb-2">{t('detail.hostUpdates.scheduleTime')}</Label>
          <Input
            type="time"
            value={time}
            disabled={disabled || !enabled}
            onChange={(evt) => setTime(evt.target.value)}
          />
        </div>

        <div>
          <Label className="mb-2">{t('detail.hostUpdates.scheduleDays')}</Label>
          <div className="flex flex-wrap gap-3">
            {DAYS.map((day) => (
              <label key={day} className="flex items-center gap-2 text-sm text-foreground">
                <Checkbox
                  checked={days.includes(day)}
                  disabled={disabled || !enabled}
                  aria-label={dayLabels[day]}
                  onCheckedChange={(next) => toggleDay(day, next === true)}
                />
                {dayLabels[day]}
              </label>
            ))}
          </div>
        </div>

        <div className="border-t border-border pt-4">
          <div className="flex items-start gap-3">
            <Switch
              id="host-update-new-packages"
              checked={withNewPackages}
              disabled={disabled}
              onCheckedChange={setWithNewPackages}
            />
            <div className="min-w-0">
              <Label htmlFor="host-update-new-packages" className="text-sm font-normal">
                {t('detail.hostUpdates.newPackages')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">{t('detail.hostUpdates.newPackagesDescription')}</p>
            </div>
          </div>
          <div className="mt-4 flex items-start gap-3">
            <Switch
              id="host-update-skip-unavailable"
              checked={skipUnavailable}
              disabled={disabled}
              onCheckedChange={setSkipUnavailable}
            />
            <div className="min-w-0">
              <Label htmlFor="host-update-skip-unavailable" className="text-sm font-normal">
                {t('detail.hostUpdates.skipUnavailable')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t('detail.hostUpdates.skipUnavailableDescription')}
              </p>
              {recoveryUnsupported ? (
                <p className="mt-1 text-xs text-warning">{t('detail.hostUpdates.recoveryUnsupported')}</p>
              ) : null}
            </div>
          </div>
          <div className="mt-4 flex items-start gap-3">
            <Switch
              id="host-update-remove-unused"
              checked={removeUnused}
              disabled={disabled}
              onCheckedChange={setRemoveUnused}
            />
            <div className="min-w-0">
              <Label htmlFor="host-update-remove-unused" className="text-sm font-normal">
                {t('detail.hostUpdates.removeUnused')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">{t('detail.hostUpdates.removeUnusedDescription')}</p>
            </div>
          </div>
          <p className="mt-4 text-xs text-muted-foreground">{t('detail.hostUpdates.refreshNote')}</p>
        </div>

        <div className="flex justify-end">
          <Button
            size="sm"
            permission={hostPermission(target, 'updatesManage')}
            disabled={disabled || save.isPending || (enabled && days.length === 0)}
            onClick={() => save.mutate({ enabled, time, days, withNewPackages, skipUnavailable, removeUnused })}
          >
            {save.isPending ? <Spinner className="mr-1.5 h-4 w-4" /> : null}
            {t('actions.save', { ns: 'common' })}
          </Button>
        </div>
      </div>
    </div>
  );
}
