// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ValueTree } from './ValueTree';

type TraceSectionProps = {
  label: string;
  badge?: string;
  badgeColor?: string;
  data: unknown;
  defaultExpanded?: boolean;
};

export function TraceSection({ label, badge, badgeColor = 'bg-blue-500/20 text-blue-400', data, defaultExpanded = false }: TraceSectionProps) {
  const { t } = useTranslation('common');
  const [expanded, setExpanded] = useState(defaultExpanded);

  const isEmpty = data === null || data === undefined ||
    (typeof data === 'object' && !Array.isArray(data) && Object.keys(data as Record<string, unknown>).length === 0);

  return (
    <div className="border-t border-white/5">
      <Button
        variant="ghost"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center justify-start gap-1.5 rounded-none px-3 py-1.5 text-[10px] h-auto"
      >
        {expanded ? (
          <IconChevronDown className="size-2.5 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronRight className="size-2.5 shrink-0 text-muted-foreground" />
        )}
        <span className="font-medium text-muted-foreground">{label}</span>
        {badge && (
          <span className={`rounded px-1 py-0.5 text-[8px] font-medium ${badgeColor}`}>
            {badge}
          </span>
        )}
      </Button>
      {expanded && (
        <div className="px-3 pb-2 font-mono text-[9px] leading-relaxed">
          {isEmpty ? (
            <span className="text-muted-foreground/40 italic">{t('workflows.noConfig')}</span>
          ) : (
            <ValueTree data={data} depth={0} maxAutoExpand={2} />
          )}
        </div>
      )}
    </div>
  );
}

