// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';

const SHORTCUTS = [
  { keys: 'Delete / Backspace', descKey: 'workflows.helpDeleteSelected' },
  { keys: 'Ctrl + Z', descKey: 'workflows.helpUndo' },
  { keys: 'Ctrl + Y', descKey: 'workflows.helpRedo' },
  { keys: 'Ctrl + S', descKey: 'workflows.helpSaveWorkflow' },
  { keys: 'Shift + drag', descKey: 'workflows.helpPanCanvas' },
  { keys: 'Scroll', descKey: 'workflows.helpZoomInOut' },
] as const;

const EXPRESSION_VARIABLES = [
  { variable: 'msg.payload', color: 'text-chart-3', descKey: 'workflows.helpMsgPayload' },
  { variable: 'msg.topic', color: 'text-chart-1', descKey: 'workflows.helpMsgTopic' },
  { variable: 'msg._msgid', color: 'text-warning', descKey: 'workflows.helpMsgId' },
  { variable: 'flow.*', color: 'text-success', descKey: 'workflows.helpFlowVars' },
  { variable: 'global.*', color: 'text-chart-3', descKey: 'workflows.helpGlobalVars' },
] as const;

export function HelpTabGeneralHelp() {
  const { t } = useTranslation('common');

  return (
    <div className="space-y-5 p-4">
      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpAddingNodes')}</h3>
        <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">{t('workflows.helpAddingNodesDesc')}</p>
      </section>

      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpConnectingNodes')}</h3>
        <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
          {t('workflows.helpConnectingNodesDesc')}
        </p>
      </section>

      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpKeyboardShortcuts')}</h3>
        <div className="mt-1.5 space-y-1">
          {SHORTCUTS.map(({ keys, descKey }) => (
            <div key={keys} className="flex items-center gap-2">
              <kbd className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">{keys}</kbd>
              <span className="text-[11px] text-muted-foreground">{t(descKey)}</span>
            </div>
          ))}
        </div>
      </section>

      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpExpressionVariables')}</h3>
        <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
          {t('workflows.helpExpressionVariablesDesc')}
        </p>
        <div className="mt-2 space-y-1.5">
          {EXPRESSION_VARIABLES.map(({ variable, color, descKey }) => (
            <div key={variable}>
              <code className={cn('font-mono text-[10px]', color)}>{`{{ ${variable} }}`}</code>
              <p className="text-[10px] text-muted-foreground">{t(descKey)}</p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
