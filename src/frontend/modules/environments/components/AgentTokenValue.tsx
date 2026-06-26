// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

type AgentTokenValueProps = {
  label: string;
  token: string;
  copiedKey: string | null;
  ariaLabel: string;
  onCopy: (value: string, key: string, label: string) => void;
};

export function AgentTokenValue({
  label,
  token,
  copiedKey,
  ariaLabel,
  onCopy,
}: AgentTokenValueProps) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-muted-foreground">{label}</label>
      <div className="flex items-center gap-2">
        <code className="flex-1 rounded border border-border bg-muted px-3 py-2 font-mono text-xs break-all">
          {token}
        </code>
        <Button
          variant="outline"
          size="icon"
          onClick={() => onCopy(token, 'token', label)}
          aria-label={ariaLabel}
        >
          {copiedKey === 'token' ? (
            <IconCheck className="h-4 w-4 text-green-500" />
          ) : (
            <IconCopy className="h-4 w-4" />
          )}
        </Button>
      </div>
    </div>
  );
}
