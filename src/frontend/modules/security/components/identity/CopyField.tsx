// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { useFlash } from '@resources/hooks/useFlash';
import { copyToClipboard } from '@resources/utils/clipboard';

type CopyFieldProps = {
  label: string;
  value: string;
  hint?: string;
};

export function CopyField({ label, value, hint }: CopyFieldProps) {
  const { t } = useTranslation('common');
  const { flashing: copied, flash: flashCopied } = useFlash();
  const [manual, setManual] = useState(false);

  const copy = async () => {
    const result = await copyToClipboard(value);
    if (!result.ok) {
      setManual(true);
      return;
    }
    flashCopied();
  };

  return (
    <div>
      <p className="text-sm font-medium text-foreground">{label}</p>
      <div className="mt-1 flex items-center gap-2 rounded-md bg-muted px-3 py-2">
        <code className="min-w-0 flex-1 text-xs break-all text-foreground">{value}</code>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`${t('clipboard.copy')}: ${label}`}
          onClick={() => void copy()}
        >
          {copied ? <IconCheck className="size-4 text-success" /> : <IconCopy className="size-4" />}
        </Button>
      </div>
      {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
      <ManualCopyDialog open={manual} onOpenChange={setManual} value={value} title={label} />
    </div>
  );
}
