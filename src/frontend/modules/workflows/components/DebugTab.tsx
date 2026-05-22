// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBug, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NODE_DEFINITION_MAP, CATEGORY_TAG_COLORS } from '../nodes';
import { TraceSection } from './TraceSection';

export type NodeTrace = {
  id: string;
  type: 'trace';
  timestamp: string;
  nodeId: string;
  nodeLabel: string;
  action: string;
  durationMs: number;
  outputPort: string;
  input: unknown;
  config: unknown;
  output: unknown;
};

export type DebugMessage = {
  id: string;
  type: 'debug';
  timestamp: string;
  source: 'debug-node' | 'sniffer';
  nodeId?: string;
  nodeLabel?: string;
  level?: string;
  message?: string;
  data?: unknown;
};

export type DebugEntry = NodeTrace | DebugMessage;

type DebugTabProps = {
  messages: DebugEntry[];
  onClear: () => void;
};

export function DebugTab({ messages, onClear }: DebugTabProps) {
  const { t } = useTranslation('common');

  if (messages.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-4">
        <IconBug className="size-8 text-muted-foreground/20" />
        <p className="mt-3 text-xs text-muted-foreground">{t('workflows.noDebugMessages')}</p>
        <p className="mt-1 text-[10px] text-muted-foreground/60">
          {t('workflows.noDebugDescription')}
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <span className="text-[10px] text-muted-foreground">{t('workflows.entries', { count: messages.length })}</span>
        <Button variant="link" onClick={onClear} className="h-auto p-0 text-[10px]">
          {t('workflows.clear')}
        </Button>
      </div>
      <div className="flex-1 overflow-y-auto">
        {messages.map((entry) => (
          entry.type === 'trace'
            ? <NodeTraceRow key={entry.id} entry={entry} />
            : <DebugMessageRow key={entry.id} entry={entry} />
        ))}
      </div>
    </div>
  );
}

function NodeTraceRow({ entry }: { entry: NodeTrace }) {
  const { t } = useTranslation('common');
  const [expanded, setExpanded] = useState(false);
  const time = entry.timestamp.split('T')[1]?.split('.')[0] ?? entry.timestamp;
  const def = NODE_DEFINITION_MAP[entry.action];
  const tagColor = def ? CATEGORY_TAG_COLORS[def.category] ?? '' : 'bg-blue-500/20 text-blue-400';

  return (
    <div className="border-b border-border">
      <Button
        variant="ghost"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-1.5 rounded-none px-3 py-2 h-auto"
      >
        {expanded ? (
          <IconChevronDown className="size-2.5 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronRight className="size-2.5 shrink-0 text-muted-foreground" />
        )}
        <span className="shrink-0 font-mono text-[9px] text-muted-foreground/50">{time}</span>
        <span className={`shrink-0 rounded px-1.5 py-0.5 text-[8px] font-medium ${tagColor}`}>
          {entry.nodeLabel}
        </span>
        <span className="flex-1" />
        <span className="text-[9px] text-muted-foreground/40">{entry.durationMs}ms</span>
      </Button>
      {expanded && (
        <div>
          <TraceSection
            label={t('workflows.input')}
            badge="input"
            badgeColor="bg-slate-500/20 text-slate-400"
            data={entry.input}
          />
          <TraceSection
            label={t('workflows.config')}
            data={entry.config}
          />
          <TraceSection
            label={t('workflows.output')}
            badge={entry.outputPort}
            badgeColor="bg-emerald-500/20 text-emerald-400"
            data={entry.output}
          />
        </div>
      )}
    </div>
  );
}

function DebugMessageRow({ entry }: { entry: DebugMessage }) {
  const { t } = useTranslation('common');
  const [expanded, setExpanded] = useState(false);
  const hasData = entry.data !== undefined && entry.data !== null;
  const time = entry.timestamp.split('T')[1]?.split('.')[0] ?? entry.timestamp;

  const sourceLabel = entry.source === 'debug-node' ? 'debug' : 'sniff';
  const sourceBadgeColor = entry.source === 'sniffer'
    ? 'bg-red-500/20 text-red-400'
    : 'bg-purple-500/20 text-purple-400';

  return (
    <div className="border-b border-border">
      <Button
        variant="ghost"
        onClick={() => hasData && setExpanded(!expanded)}
        className="flex w-full items-center gap-1.5 rounded-none px-3 py-2 h-auto"
      >
        {hasData ? (
          expanded
            ? <IconChevronDown className="size-2.5 shrink-0 text-muted-foreground" />
            : <IconChevronRight className="size-2.5 shrink-0 text-muted-foreground" />
        ) : (
          <span className="size-2.5 shrink-0" />
        )}
        <span className="shrink-0 font-mono text-[9px] text-muted-foreground/50">{time}</span>
        <span className={`shrink-0 rounded px-1 py-0.5 text-[8px] font-medium ${sourceBadgeColor}`}>
          {sourceLabel}
        </span>
        {entry.level && entry.level !== 'info' && (
          <span className={`shrink-0 rounded px-1 py-0.5 text-[8px] font-medium ${
            entry.level === 'error' ? 'bg-red-500/20 text-red-400' :
            entry.level === 'warning' ? 'bg-amber-500/20 text-amber-400' :
            'bg-blue-500/20 text-blue-400'
          }`}>
            {entry.level}
          </span>
        )}
        {entry.nodeLabel && (
          <span className="truncate text-[10px] font-medium text-foreground">{entry.nodeLabel}</span>
        )}
        {entry.message && (
          <span className="truncate flex-1 text-left text-[10px] text-muted-foreground">{entry.message}</span>
        )}
      </Button>
      {hasData && expanded && (
        <TraceSection
          label={t('workflows.data')}
          badge={sourceLabel}
          badgeColor={sourceBadgeColor}
          data={entry.data}
          defaultExpanded
        />
      )}
    </div>
  );
}

