// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Switch } from '@resources/components/ui/Switch';
import { ScheduleFields } from './ScheduleFields';
import { emptyScheduleDraft } from './schedule-draft';
import type { ScheduleDraft } from './schedule-draft';

// "Schedule this report" — the block inside the configure dialog, and the
// fields the schedules page's edit dialog reuses.
//
// The studio wrapper adds one thing the edit dialog does not need: an
// opt-in switch, because in the studio a schedule is an alternative to
// generating now. Everything below that switch is `ScheduleFields`, shared
// by both — the cron presets, the path rules and the "link only" labelling
// all have to agree in both places or one of them is lying.
//
// The frequency control is a preset plus a time rather than a cron field,
// because "every Monday at 07:00" is what an operator actually wants and
// `0 7 * * 1` is what they would have to get right to say it. The cron
// escape hatch is still there for the cases the presets do not cover, and
// the derived expression is shown either way so the two never disagree.

type Props = {
  /** Null when scheduling is switched off. */
  draft: ScheduleDraft | null;
  onChange: (next: ScheduleDraft | null) => void;
  defaultName: string;
};

export function ScheduleSection({ draft, onChange, defaultName }: Props) {
  const { t } = useTranslation('reporting');

  return (
    <div className="rounded-lg border border-border bg-muted/20 p-4">
      <div className="flex items-start gap-3">
        <Switch
          id="reporting-schedule-enable"
          checked={draft !== null}
          onCheckedChange={(next) => onChange(next ? emptyScheduleDraft(defaultName) : null)}
          aria-label={t('schedule.enable')}
          className="mt-0.5"
        />
        <div className="min-w-0">
          <label htmlFor="reporting-schedule-enable" className="cursor-pointer text-sm font-medium text-foreground">
            {t('schedule.enable')}
          </label>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('schedule.enableHint')}</p>
        </div>
      </div>

      {draft ? (
        <div className="mt-4">
          <ScheduleFields draft={draft} onChange={onChange} />
        </div>
      ) : null}
    </div>
  );
}
