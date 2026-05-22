// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { useCanvasStore } from '../stores/canvasStore';
import type { CanvasNode, SwitchCase } from '../types';

interface SwitchCasesProps {
  node: CanvasNode;
}

export function SwitchCases({ node }: SwitchCasesProps) {
  const { t } = useTranslation('common');
  const updateNodeConfig = useCanvasStore((s) => s.updateNodeConfig);
  const cases = (node.config.switch_cases ?? []) as SwitchCase[];
  const caseKeys = useStableListKeys(cases, (switchCase) => `${switchCase.label}\u0000${switchCase.value}`);

  const setCases = (next: SwitchCase[]) => {
    updateNodeConfig(node.id, { ...node.config, switch_cases: next });
  };

  const addCase = () => {
    setCases([...cases, { value: '', label: `Case ${cases.length + 1}` }]);
  };

  const removeCase = (idx: number) => {
    setCases(cases.filter((_, i) => i !== idx));
  };

  const updateCase = (idx: number, patch: Partial<SwitchCase>) => {
    setCases(cases.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  };

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <Label className="text-xs">{t('workflows.switchCases')}</Label>
        <Button size="sm" variant="ghost" onClick={addCase} className="h-6 px-2 text-[10px]">
          {t('workflows.addCase')}
        </Button>
      </div>
      {cases.length === 0 && (
        <p className="text-[10px] text-muted-foreground">{t('workflows.noSwitchCases')}</p>
      )}
      <div className="space-y-2">
        {cases.map((sc, i) => (
          <div key={caseKeys[i]} className="flex items-center gap-1.5 rounded-md border border-border p-2">
            <span className="shrink-0 text-[10px] font-medium text-emerald-400/70">#{i}</span>
            <Input
              type="text"
              value={sc.value}
              onChange={(e) => updateCase(i, { value: e.target.value })}
              placeholder={t('workflows.switchCaseValue')}
              className="h-7 flex-1 text-xs font-mono"
            />
            <Input
              type="text"
              value={sc.label}
              onChange={(e) => updateCase(i, { label: e.target.value })}
              placeholder={t('workflows.switchCaseLabel')}
              className="h-7 w-24 shrink-0 text-xs"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={() => removeCase(i)}
              aria-label={t('common.remove')}
              className="shrink-0 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <IconX className="size-3.5" />
            </Button>
          </div>
        ))}
      </div>
      <p className="mt-1.5 text-[10px] text-muted-foreground/60">{t('workflows.switchDefaultHint')}</p>
    </div>
  );
}

