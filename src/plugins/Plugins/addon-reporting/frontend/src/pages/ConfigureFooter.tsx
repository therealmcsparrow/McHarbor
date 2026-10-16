// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import type { ConfigureState } from './useConfigureState';
import type { ReportType } from '../types';

type Props = {
  type: ReportType;
  state: ConfigureState;
  pending: boolean;
  error: boolean;
  onClose: () => void;
};

export function ConfigureFooter({ type, state, pending, error, onClose }: Props) {
  const { t } = useTranslation('reporting');
  const { isAdHoc, promptText, subject, schedule, selectedEnv, rangeInvalid, scheduleInvalid, aiUnavailable, submit } =
    state;

  return (
    <>
      <p className={cn('text-xs', error || rangeInvalid ? 'text-danger' : 'text-muted-foreground')}>
        {rangeInvalid
          ? t('studio.rangeInvalid')
          : scheduleInvalid
            ? t('schedule.incomplete')
            : error
              ? t('studio.error')
              : subject
                ? t('studio.scopeSubject', { subject: subject.name })
                : type.environment_scoped && selectedEnv?.name
                  ? t('studio.scope', { environment: selectedEnv.name })
                  : t('studio.scopeAll')}
      </p>
      <div className="flex items-center gap-2">
        <Button variant="ghost" onClick={onClose} disabled={pending}>
          {t('action.cancel')}
        </Button>
        <Button
          onClick={submit}
          permission={schedule ? 'reporting.schedule' : 'reporting.generate'}
          disabled={
            pending ||
            rangeInvalid ||
            scheduleInvalid ||
            (isAdHoc && promptText.trim() === '') ||
            // Ad-hoc has no non-AI path, so a confirmed absence of any
            // usable connection blocks it outright. Every other type
            // renders fine without AI — narrative polish just falls
            // back to its deterministic wording — so it is never
            // blocked by this.
            (isAdHoc && aiUnavailable)
          }
        >
          {pending ? <Spinner className="size-4" /> : null}
          {pending ? t('studio.generating') : schedule ? t('schedule.create') : t('studio.generate')}
        </Button>
      </div>
    </>
  );
}
