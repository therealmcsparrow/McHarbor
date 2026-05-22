// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';

export type MetricCondition = {
  metric: string;
  operator: string;
  threshold: number;
};

const METRIC_OPTIONS = [
  { value: 'cpu_percent', label: 'CPU %' },
  { value: 'mem_percent', label: 'Memory %' },
  { value: 'mem_usage_mb', label: 'Memory (MB)' },
  { value: 'net_rx', label: 'Net RX (bytes)' },
  { value: 'net_tx', label: 'Net TX (bytes)' },
  { value: 'block_read', label: 'Block Read (bytes)' },
  { value: 'block_write', label: 'Block Write (bytes)' },
  { value: 'pids', label: 'PIDs' },
];

const METRIC_OPERATORS = [
  { value: '>', label: '>' },
  { value: '<', label: '<' },
  { value: '>=', label: '>=' },
  { value: '<=', label: '<=' },
  { value: '==', label: '==' },
];

interface MetricConditionsFieldProps {
  value: unknown;
  onChange: (v: unknown) => void;
}

export function MetricConditionsField({ value, onChange }: MetricConditionsFieldProps) {
  const { t } = useTranslation('common');
  const conditions = (Array.isArray(value) ? value : []) as MetricCondition[];
  const conditionKeys = useStableListKeys(
    conditions,
    (condition) => `${condition.metric}\u0000${condition.operator}\u0000${condition.threshold}`,
  );

  const setConditions = (next: MetricCondition[]) => onChange(next);

  const addCondition = () => {
    setConditions([...conditions, { metric: 'cpu_percent', operator: '>', threshold: 80 }]);
  };

  const removeCondition = (idx: number) => {
    setConditions(conditions.filter((_, i) => i !== idx));
  };

  const updateCondition = (idx: number, patch: Partial<MetricCondition>) => {
    setConditions(conditions.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  };

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <Label className="text-xs">{t('workflows.metricConditions')}</Label>
        <Button size="sm" variant="ghost" onClick={addCondition} className="h-6 px-2 text-[10px]">
          {t('workflows.addMetricCondition')}
        </Button>
      </div>
      {conditions.length === 0 && (
        <p className="text-[10px] text-muted-foreground">{t('workflows.noMetricConditions')}</p>
      )}
      <div className="space-y-2">
        {conditions.map((cond, i) => (
          <div key={conditionKeys[i]} className="flex items-center gap-2">
            <select
              value={cond.metric}
              onChange={(e) => updateCondition(i, { metric: e.target.value })}
              className="h-8 flex-1 min-w-0 rounded-md border border-input bg-card px-2 text-xs"
            >
              {METRIC_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
            <select
              value={cond.operator}
              onChange={(e) => updateCondition(i, { operator: e.target.value })}
              className="h-8 w-14 shrink-0 rounded-md border border-input bg-card px-1 text-xs"
            >
              {METRIC_OPERATORS.map((op) => (
                <option key={op.value} value={op.value}>{op.label}</option>
              ))}
            </select>
            <NumberInput
              value={cond.threshold}
              onChange={(v) => updateCondition(i, { threshold: v })}
              size="sm"
              className="w-24 shrink-0"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={() => removeCondition(i)}
              aria-label="Remove condition"
              className="shrink-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <IconX className="size-3.5" />
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}

