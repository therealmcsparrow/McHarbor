// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Button } from '@resources/components/ui/Button';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { useCanvasStore } from '../stores/canvasStore';
import type { CanvasNode, ExtraCondition } from '../types';
import { PREDICATE_OPERATORS } from '../node-predicates';

// Symbolic operators read better unlabelled; the word-shaped ones are translated.
const OPERATOR_LABEL_KEYS: Partial<Record<string, string>> = {
  contains: 'workflows.operatorContains',
  starts_with: 'workflows.operatorStartsWith',
  regex: 'workflows.operatorRegex',
  is_empty: 'workflows.operatorIsEmpty',
  is_not_empty: 'workflows.operatorIsNotEmpty',
};

interface ExtraConditionsProps {
  node: CanvasNode;
}

export function ExtraConditions({ node }: ExtraConditionsProps) {
  const { t } = useTranslation('common');
  const updateNodeConfig = useCanvasStore((s) => s.updateNodeConfig);
  const extras = (node.config.extra_conditions ?? []) as ExtraCondition[];
  const extraKeys = useStableListKeys(
    extras,
    (condition) => `${condition.label}\u0000${condition.field}\u0000${condition.operator}\u0000${condition.value}`,
  );

  // Every operator here must be one evaluateCondition implements, or the
  // condition silently always passes. PREDICATE_OPERATORS is that set, and
  // node-predicates.contract.test.ts parses the Go switch to keep them in step.
  const OPERATORS = PREDICATE_OPERATORS.map((operator) => ({
    value: operator.value,
    label: OPERATOR_LABEL_KEYS[operator.value] ? t(OPERATOR_LABEL_KEYS[operator.value]!) : operator.value,
  }));

  const setExtras = (next: ExtraCondition[]) => {
    updateNodeConfig(node.id, { ...node.config, extra_conditions: next });
  };

  const addCondition = () => {
    setExtras([
      ...extras,
      { field: '', operator: '==', value: '', label: t('workflows.conditionLabel', { count: extras.length + 1 }) },
    ]);
  };

  const removeCondition = (idx: number) => {
    setExtras(extras.filter((_, i) => i !== idx));
  };

  const updateCondition = (idx: number, patch: Partial<ExtraCondition>) => {
    setExtras(extras.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  };

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <Label className="text-xs">{t('workflows.extraConditions')}</Label>
        <Button size="sm" variant="ghost" onClick={addCondition} className="px-2 text-[10px]">
          {t('workflows.addCondition')}
        </Button>
      </div>
      {extras.length === 0 && <p className="text-[10px] text-muted-foreground">{t('workflows.noExtraConditions')}</p>}
      <div className="space-y-2">
        {extras.map((cond, i) => (
          <div key={extraKeys[i]} className="space-y-1.5 rounded-md border border-border p-2">
            <div className="flex items-center gap-1.5">
              <Input
                type="text"
                value={cond.label}
                onChange={(e) => updateCondition(i, { label: e.target.value })}
                placeholder={t('workflows.labelField')}
                className="flex-1 text-xs"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                onClick={() => removeCondition(i)}
                aria-label={t('workflows.removeCondition')}
                className="shrink-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              >
                <IconX className="size-3.5" />
              </Button>
            </div>
            <div className="grid grid-cols-3 gap-1.5">
              <Input
                type="text"
                value={cond.field}
                onChange={(e) => updateCondition(i, { field: e.target.value })}
                placeholder={t('workflows.field')}
                className="font-mono text-xs"
              />
              <Select
                value={cond.operator}
                onChange={(next) => updateCondition(i, { operator: next })}
                options={OPERATORS}
                searchable={false}
                ariaLabel={t('workflows.operator')}
                variant="outline"
                className="px-1.5 text-xs"
              />
              <Input
                type="text"
                value={cond.value}
                onChange={(e) => updateCondition(i, { value: e.target.value })}
                placeholder={t('workflows.value')}
                className="font-mono text-xs"
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
