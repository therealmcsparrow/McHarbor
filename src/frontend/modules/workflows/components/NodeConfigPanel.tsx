// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy, IconTrash, IconBug, IconPlayerTrackNext, IconBan } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { useCanvasStore } from '../stores/canvasStore';
import { displayValue } from '../utils/display-value';
import { getEffectiveInputPorts, getEffectiveOutputPorts } from '../types';
import type { CanvasNode } from '../types';
import { useNodeDefinition, CATEGORY_TAG_COLORS } from '../nodes';
import { ConfigFieldRenderer } from './ConfigFieldRenderer';
import { ExtraConditions } from './ExtraConditions';
import { SwitchCases } from './SwitchCases';
import { NodePortLabels } from './NodePortLabels';
import { PERM } from '@core/auth/permissionKeys';

interface NodeConfigPanelProps {
  node: CanvasNode;
}

export function NodeConfigPanel({ node }: NodeConfigPanelProps) {
  const { t } = useTranslation('common');
  const definition = useNodeDefinition(node.action);
  const updateNodeLabel = useCanvasStore((s) => s.updateNodeLabel);
  const updateNodeConfig = useCanvasStore((s) => s.updateNodeConfig);
  const updateNodePortLabels = useCanvasStore((s) => s.updateNodePortLabels);
  const updateNodeDebug = useCanvasStore((s) => s.updateNodeDebug);
  const updateNodeSkip = useCanvasStore((s) => s.updateNodeSkip);
  const updateNodeDisabled = useCanvasStore((s) => s.updateNodeDisabled);
  const duplicateNode = useCanvasStore((s) => s.duplicateNode);
  const removeNode = useCanvasStore((s) => s.removeNode);

  const tagColor = CATEGORY_TAG_COLORS[definition?.category ?? 'action'] ?? '';
  const outputPorts = useMemo(() => getEffectiveOutputPorts(node, definition), [node, definition]);
  const inputPorts = useMemo(() => getEffectiveInputPorts(node, definition), [node, definition]);

  const setConfigValue = useCallback(
    (key: string, value: unknown) => {
      updateNodeConfig(node.id, { ...node.config, [key]: value });
    },
    [node.id, node.config, updateNodeConfig],
  );

  const setPortLabel = useCallback(
    (port: string, label: string) => {
      updateNodePortLabels(node.id, {
        ...node.portLabels,
        [port]: label,
      });
    },
    [node.id, node.portLabels, updateNodePortLabels],
  );

  const defaultConfig = useMemo(() => {
    const defaults: Record<string, unknown> = {};
    for (const field of definition?.configSchema ?? []) {
      if (field.default !== undefined) {
        defaults[field.key] = field.default;
      }
    }
    return defaults;
  }, [definition]);

  return (
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <span className={cn('rounded-md px-2 py-0.5 text-[10px] font-medium uppercase', tagColor)}>
          {definition?.category ?? node.type}
        </span>
        <span className="truncate text-xs text-muted-foreground">{definition?.label ?? node.action}</span>
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        {/* Node label */}
        <div>
          <Label className="mb-1.5 text-xs">{t('workflows.labelField')}</Label>
          <Input
            type="text"
            value={node.label}
            onChange={(e) => updateNodeLabel(node.id, e.target.value)}
            className="text-xs"
          />
        </div>

        {/* Config fields */}
        {definition?.configSchema.map((field) => {
          if (field.showWhen) {
            const show = Object.entries(field.showWhen).every(([k, v]) => {
              const current = displayValue(node.config[k] ?? defaultConfig[k]);
              return Array.isArray(v) ? v.includes(current) : current === v;
            });
            if (!show) return null;
          }
          return (
            <ConfigFieldRenderer
              key={field.key}
              field={field}
              value={node.config[field.key]}
              onChange={(v) => setConfigValue(field.key, v)}
              nodeConfig={node.config}
              nodeKey={node.action}
            />
          );
        })}

        {/* Extra conditions (for condition nodes) */}
        {node.action === 'condition' && <ExtraConditions node={node} />}

        {/* Switch cases (for switch nodes) */}
        {node.action === 'switch' && <SwitchCases node={node} />}

        {/* Port labels */}
        <NodePortLabels node={node} inputPorts={inputPorts} outputPorts={outputPorts} onLabelChange={setPortLabel} />

        {/* Node behavior toggles */}
        <div className="space-y-2 border-t border-border pt-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-1.5">
              <IconPlayerTrackNext className="size-3.5 text-warning" />
              <Label className="text-xs">{t('workflows.skipNode')}</Label>
            </div>
            <Switch
              aria-label={t('workflows.skipNode')}
              checked={node.skip}
              onCheckedChange={(checked) => updateNodeSkip(node.id, checked)}
              className={node.skip ? 'data-[state=checked]:bg-warning' : undefined}
            />
          </div>
          <p className="ml-5 text-[10px] text-muted-foreground/60">{t('workflows.skipNodeDescription')}</p>

          <div className="flex items-center justify-between">
            <div className="flex items-center gap-1.5">
              <IconBan className="size-3.5 text-danger" />
              <Label className="text-xs">{t('workflows.disableNode')}</Label>
            </div>
            <Switch
              aria-label={t('workflows.disableNode')}
              checked={node.disabled}
              onCheckedChange={(checked) => updateNodeDisabled(node.id, checked)}
              className={node.disabled ? 'data-[state=checked]:bg-danger' : undefined}
            />
          </div>
          <p className="ml-5 text-[10px] text-muted-foreground/60">{t('workflows.disableNodeDescription')}</p>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 border-t border-border pt-4">
          <Button
            size="sm"
            variant="outline"
            onClick={() => updateNodeDebug(node.id, !node.debug)}
            className={cn(node.debug && 'border-warning/50 text-warning')}
          >
            <IconBug className="size-3.5" />
            {node.debug ? t('workflows.debugOn') : t('workflows.debug')}
          </Button>
          <div className="flex-1" />
          <Button permission={PERM.WORKFLOWS_MANAGE} size="sm" variant="outline" onClick={() => duplicateNode(node.id)}>
            <IconCopy className="size-3.5" />
          </Button>
          <Button
            permission={PERM.WORKFLOWS_MANAGE}
            size="sm"
            variant="destructive"
            onClick={() => removeNode(node.id)}
          >
            <IconTrash className="size-3.5" />
          </Button>
        </div>
      </div>
    </div>
  );
}
