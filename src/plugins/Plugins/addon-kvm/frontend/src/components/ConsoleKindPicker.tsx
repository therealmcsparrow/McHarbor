// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import type { ConsoleKind } from '../types';

const KINDS: ConsoleKind[] = ['vnc', 'serial', 'spice'];

/**
 * Which console to open. SPICE stays visible when the host cannot serve it,
 * disabled with the reason, so the operator learns what their host is missing
 * rather than wondering whether McHarbor has a SPICE console at all.
 */
export function ConsoleKindPicker({
  kind,
  onChange,
  spiceSupported,
  spiceReason,
}: {
  kind: ConsoleKind;
  onChange: (kind: ConsoleKind) => void;
  spiceSupported: boolean;
  spiceReason: string;
}) {
  const { t } = useTranslation('vm');

  return (
    <fieldset
      aria-label={t('console.kind.label')}
      className="m-0 inline-flex min-w-0 gap-1 rounded-md border-0 bg-muted/40 p-0.5"
    >
      {KINDS.map((entry) => {
        const blocked = entry === 'spice' && !spiceSupported;
        const button = (
          <Button
            size="sm"
            variant={kind === entry ? 'default' : 'ghost'}
            aria-pressed={kind === entry}
            disabled={blocked}
            onClick={() => onChange(entry)}
          >
            {t(`console.kind.${entry}`)}
          </Button>
        );
        if (!blocked) return <span key={entry}>{button}</span>;
        return (
          <Tooltip key={entry}>
            <TooltipTrigger asChild>
              <TooltipFocusTarget>{button}</TooltipFocusTarget>
            </TooltipTrigger>
            <TooltipContent className="max-w-[320px]">{spiceReason}</TooltipContent>
          </Tooltip>
        );
      })}
    </fieldset>
  );
}
