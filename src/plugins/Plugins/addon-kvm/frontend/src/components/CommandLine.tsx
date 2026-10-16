// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

/** A command the operator runs elsewhere, with a copy button. */
export function CommandLine({ command }: { command: string }) {
  const { t } = useTranslation('vm');
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(false), 2000);
    return () => window.clearTimeout(timer);
  }, [copied]);

  return (
    <div className="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1.5">
      <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">{command}</code>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('guestAgent.install.copy')}
        onClick={() => {
          void navigator.clipboard.writeText(command).then(() => setCopied(true));
        }}
      >
        {copied ? (
          <IconCheck className="h-4 w-4 text-success" aria-hidden="true" />
        ) : (
          <IconCopy className="h-4 w-4" aria-hidden="true" />
        )}
      </Button>
    </div>
  );
}
