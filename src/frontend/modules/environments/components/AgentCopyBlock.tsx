// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

type AgentCopyBlockProps = {
  label: string;
  value: string;
  copyKey: string;
  copiedKey: string | null;
  ariaLabel: string;
  onCopy: (value: string, key: string, label: string) => void;
  note?: string;
};

export function AgentCopyBlock({
  label,
  value,
  copyKey,
  copiedKey,
  ariaLabel,
  onCopy,
  note,
}: AgentCopyBlockProps) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-muted-foreground">{label}</label>
      <div className="relative">
        <pre className="rounded border border-border bg-muted p-3 font-mono text-xs overflow-x-auto whitespace-pre">
          {value}
        </pre>
        <Button
          variant="ghost"
          size="icon"
          className="absolute right-1 top-1"
          onClick={() => onCopy(value, copyKey, label)}
          aria-label={ariaLabel}
        >
          {copiedKey === copyKey ? (
            <IconCheck className="h-3.5 w-3.5 text-green-500" />
          ) : (
            <IconCopy className="h-3.5 w-3.5" />
          )}
        </Button>
      </div>
      {note && <p className="mt-1 text-xs text-muted-foreground">{note}</p>}
    </div>
  );
}
