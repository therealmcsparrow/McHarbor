// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import type { ConsoleDisplayMode } from '../console-resize';

export function ConsoleDisplayModePicker({
  mode,
  onChange,
}: {
  mode: ConsoleDisplayMode;
  onChange: (mode: ConsoleDisplayMode) => void;
}) {
  const { t } = useTranslation('vm');
  return (
    <Select
      value={mode}
      ariaLabel={t('console.display.label')}
      searchable={false}
      className="w-auto min-w-[12rem]"
      options={[
        { value: 'resize', label: t('console.display.resize') },
        { value: 'scale', label: t('console.display.scale') },
      ]}
      onChange={(value) => onChange(value === 'scale' ? 'scale' : 'resize')}
    />
  );
}
