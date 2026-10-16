// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The audit page's own filter controls — the left half of TablePage's second
// row. Date range, the action-catalogue trigger, and the entity-type select.

import { useTranslation } from 'react-i18next';
import { IconTags } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { MultiSelect } from '@resources/components/ui/MultiSelect';
import { Select, type SelectOption } from '@resources/components/ui/Select';
import { DateRangeFilter, type DateRangeFilterValue } from '@resources/components/DateRangeFilter';
import { AUDIT_SEVERITIES } from '../hooks/useAuditCenter';

type AuditFilterControlsProps = {
  dateRange: DateRangeFilterValue;
  onDateRangeChange: (value: DateRangeFilterValue) => void;
  actionCount: number;
  onOpenActions: () => void;
  entityType: string;
  onEntityTypeChange: (value: string) => void;
  entityTypeOptions: SelectOption[];
  actor: string;
  onActorChange: (value: string) => void;
  severities: string[];
  onSeveritiesChange: (value: string[]) => void;
};

export function AuditFilterControls({
  dateRange,
  onDateRangeChange,
  actionCount,
  onOpenActions,
  entityType,
  onEntityTypeChange,
  entityTypeOptions,
  actor,
  onActorChange,
  severities,
  onSeveritiesChange,
}: AuditFilterControlsProps) {
  const { t } = useTranslation('common');

  return (
    <>
      <DateRangeFilter value={dateRange} onChange={onDateRangeChange} />
      <Button variant="outline" onClick={onOpenActions}>
        <IconTags className="size-4" />
        {actionCount === 0 ? t('audit.allActions') : t('audit.selectedActions', { count: actionCount })}
      </Button>
      <Select
        value={entityType}
        onChange={onEntityTypeChange}
        options={entityTypeOptions}
        placeholder={t('audit.filterEntityType')}
        className="w-48"
      />
      <Input
        className="w-44"
        value={actor}
        placeholder={t('filters.actorPlaceholder', { ns: 'audit' })}
        aria-label={t('filters.actor', { ns: 'audit' })}
        onChange={(evt) => onActorChange(evt.target.value)}
      />
      <MultiSelect
        value={severities}
        onChange={onSeveritiesChange}
        options={AUDIT_SEVERITIES.map((severity) => ({ value: severity, label: t(`audit.severity.${severity}`) }))}
        emptyLabel={t('filters.anySeverity', { ns: 'audit' })}
        ariaLabel={t('filters.severity', { ns: 'audit' })}
        className="w-44"
      />
    </>
  );
}
