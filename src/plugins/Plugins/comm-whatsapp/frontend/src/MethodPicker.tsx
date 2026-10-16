// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Button } from '@resources/components/ui/Button';
import { Grid } from '@resources/components/ui/Layout';
import { METHOD_OPTIONS } from './fields';
import type { WhatsappMethod } from './index';

type Props = {
  method: WhatsappMethod;
  onPick: (next: WhatsappMethod) => void;
  t: (key: string) => string;
};

export function MethodPicker({ method, onPick, t }: Props) {
  return (
    <Grid columns={2} gap={3}>
      {METHOD_OPTIONS.map((opt) => {
        const Icon = opt.icon;
        const selected = method === opt.value;
        return (
          <Button
            key={opt.value}
            type="button"
            variant="ghost"
            size="lg"
            onClick={() => onPick(opt.value)}
            aria-pressed={selected}
            className={
              'flex h-auto items-start justify-start gap-3 rounded-md border p-3 text-left font-normal whitespace-normal transition-colors ' +
              (selected
                ? 'border-primary bg-primary/10 text-foreground'
                : 'border-input bg-background text-foreground hover:border-primary/50')
            }
          >
            <Icon
              className={'mt-0.5 size-5 shrink-0 ' + (selected ? 'text-primary' : 'text-muted-foreground')}
              aria-hidden="true"
            />
            <span className="flex flex-col">
              <span className="text-sm font-medium">{t(opt.labelKey)}</span>
              <span className="text-xs text-muted-foreground">{t(opt.subKey)}</span>
            </span>
          </Button>
        );
      })}
    </Grid>
  );
}
