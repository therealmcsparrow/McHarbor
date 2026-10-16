// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { SUPPORTED_TIMEZONES } from '@resources/utils/timezones';

interface TimezoneSelectProps {
  value: string;
  onChange: (value: string) => void;
  ariaLabel: string;
}

/**
 * Searchable IANA timezone picker. Shares the bundled list the
 * environment automation tab uses, so a workflow and the environment it
 * runs in can never disagree on what counts as a valid zone. A stored
 * value outside the list (an old free-text entry) is kept as a row so it
 * stays visible instead of silently blanking the field.
 */
export function TimezoneSelect({ value, onChange, ariaLabel }: TimezoneSelectProps) {
  const { t } = useTranslation('common');

  const options = useMemo(() => {
    const base = SUPPORTED_TIMEZONES.map((zone) => ({ value: zone, label: zone }));
    if (value && !SUPPORTED_TIMEZONES.includes(value)) {
      return [{ value, label: value }, ...base];
    }
    return base;
  }, [value]);

  return (
    <Select
      value={value}
      onChange={onChange}
      options={options}
      searchable
      placeholder={t('workflows.selectTimezone')}
      ariaLabel={ariaLabel}
      variant="outline"
      className="text-xs"
    />
  );
}
