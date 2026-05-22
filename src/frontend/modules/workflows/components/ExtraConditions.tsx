// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { useCanvasStore } from '../stores/canvasStore';
import type { CanvasNode, ExtraCondition } from '../types';

interface ExtraConditionsProps {
  node: CanvasNode;
}

export function ExtraConditions({ node }: ExtraConditionsProps) {
  const { t } = useTranslation('common');
  const updateNodeConfig = useCanvasStore((s) => s.updateNodeConfig);
  const extras = (node.config.extra_conditions ?? []) as ExtraCondition[];
  const extraKeys = useStableListKeys(
    extras,
    (condition) =>
      `${condition.label}\u0000${condition.field}\u0000${condition.operator}\u0000${condition.value}`,
  );

  const OPERATORS = [
    { value: '==', label: '==' },
    { value: '!=', label: '!=' },
    { value: '>', label: '>' },
    { value: '<', label: '<' },
    { value: '>=', label: '>=' },
    { value: '<=', label: '<=' },
    { value: 'contains', label: t('workflows.operatorContains') },
    { value: 'starts_with', label: t('workflows.operatorStartsWith') },
    { value: 'is_empty', label: t('workflows.operatorIsEmpty') },
    { value: 'is_not_empty', label: t('workflows.operatorIsNotEmpty') },
  ];

  const setExtras = (next: ExtraCondition[]) => {
    updateNodeConfig(node.id, { ...node.config, extra_conditions: next });
  };

  const addCondition = () => {
    setExtras([...extras, { field: '', operator: '==', value: '', label: t('workflows.conditionLabel', { count: extras.length + 1 }) }]);
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
        <Button size="sm" variant="ghost" onClick={addCondition} className="h-6 px-2 text-[10px]">
          {t('workflows.addCondition')}
        </Button>
      </div>
      {extras.length === 0 && (
        <p className="text-[10px] text-muted-foreground">{t('workflows.noExtraConditions')}</p>
      )}
      <div className="space-y-2">
        {extras.map((cond, i) => (
          <div key={extraKeys[i]} className="rounded-md border border-border p-2 space-y-1.5">
            <div className="flex items-center gap-1.5">
              <Input
                type="text"
                value={cond.label}
                onChange={(e) => updateCondition(i, { label: e.target.value })}
                placeholder={t('workflows.labelField')}
                className="h-7 flex-1 text-xs"
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
                className="h-7 text-xs font-mono"
              />
              <select
                value={cond.operator}
                onChange={(e) => updateCondition(i, { operator: e.target.value })}
                className="h-7 rounded-md border border-input bg-card px-1.5 text-xs"
              >
                {OPERATORS.map((op) => (
                  <option key={op.value} value={op.value}>{op.label}</option>
                ))}
              </select>
              <Input
                type="text"
                value={cond.value}
                onChange={(e) => updateCondition(i, { value: e.target.value })}
                placeholder={t('workflows.value')}
                className="h-7 text-xs font-mono"
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

