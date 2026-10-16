// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { CanvasNode } from '../types';

interface NodePortLabelsProps {
  node: CanvasNode;
  inputPorts: string[];
  outputPorts: string[];
  onLabelChange: (port: string, label: string) => void;
}

export function NodePortLabels({ node, inputPorts, outputPorts, onLabelChange }: NodePortLabelsProps) {
  const { t } = useTranslation('common');

  return (
    <div>
      <Label className="mb-1.5 text-xs">{t('workflows.portLabels')}</Label>
      <div className="space-y-1.5">
        {inputPorts.map((port) => (
          <div key={`in-${port}`} className="flex items-center gap-2">
            <span className="w-16 shrink-0 text-right text-[10px] text-info/70">{`in:${port}`}</span>
            <Input
              type="text"
              value={node.portLabels?.[`in:${port}`] ?? ''}
              onChange={(e) => onLabelChange(`in:${port}`, e.target.value)}
              placeholder={port}
              className="text-xs"
            />
          </div>
        ))}
        {outputPorts.map((port) => (
          <div key={`out-${port}`} className="flex items-center gap-2">
            <span className="w-16 shrink-0 text-right text-[10px] text-success/70">{`out:${port}`}</span>
            <Input
              type="text"
              value={node.portLabels?.[`out:${port}`] ?? ''}
              onChange={(e) => onLabelChange(`out:${port}`, e.target.value)}
              placeholder={port}
              className="text-xs"
            />
          </div>
        ))}
      </div>
    </div>
  );
}
