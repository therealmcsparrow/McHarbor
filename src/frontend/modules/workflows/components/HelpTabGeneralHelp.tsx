// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';

export function HelpTabGeneralHelp() {
  const { t } = useTranslation('common');

  return (
    <div className="space-y-5 p-4">
      <section>
        <h3 className="text-xs font-semibold text-foreground">{t('workflows.helpAddingNodes')}</h3>
        <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
          {t('workflows.helpAddingNodesDesc')}
        </p>
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
          {[
            ['Delete / Backspace', t('workflows.helpDeleteSelected')],
            ['Ctrl + Z', t('workflows.helpUndo')],
            ['Ctrl + Y', t('workflows.helpRedo')],
            ['Ctrl + S', t('workflows.helpSaveWorkflow')],
            ['Shift + drag', t('workflows.helpPanCanvas')],
            ['Scroll', t('workflows.helpZoomInOut')],
          ].map(([key, desc]) => (
            <div key={key} className="flex items-center gap-2">
              <kbd className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">{key}</kbd>
              <span className="text-[11px] text-muted-foreground">{desc}</span>
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
          {[
            { ns: 'msg.payload', color: 'text-indigo-400', desc: t('workflows.helpMsgPayload') },
            { ns: 'msg.topic', color: 'text-cyan-400', desc: t('workflows.helpMsgTopic') },
            { ns: 'msg._msgid', color: 'text-amber-400', desc: t('workflows.helpMsgId') },
            { ns: 'flow.*', color: 'text-emerald-400', desc: t('workflows.helpFlowVars') },
            { ns: 'global.*', color: 'text-purple-400', desc: t('workflows.helpGlobalVars') },
          ].map(({ ns, color, desc }) => (
            <div key={ns}>
              <code className={cn('text-[10px] font-mono', color)}>{`{{ ${ns} }}`}</code>
              <p className="text-[10px] text-muted-foreground">{desc}</p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
