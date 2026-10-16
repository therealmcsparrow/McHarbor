// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Stack } from '@resources/components/ui/Layout';
import { copyToClipboard } from '@resources/utils/clipboard';
import type { Translate } from './useRtValidation';

type CopyState = 'idle' | 'copied' | 'manual';

type Props = {
  title: string;
  code: string;
  t: Translate;
};

export function ScripSnippet({ title, code, t }: Props) {
  const [state, setState] = useState<CopyState>('idle');

  const copy = async () => {
    const result = await copyToClipboard(code);
    setState(result.ok ? 'copied' : 'manual');
  };

  return (
    <Stack gap={1}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-foreground">{title}</span>
        <Button variant="ghost" size="icon" aria-label={t('snippet.copy')} onClick={() => void copy()}>
          <IconCopy size={16} aria-hidden="true" />
        </Button>
      </div>
      <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs text-foreground">{code}</pre>
      {state !== 'idle' && (
        <output className="text-xs text-muted-foreground">
          {state === 'copied' ? t('snippet.copied') : t('snippet.manualCopy')}
        </output>
      )}
    </Stack>
  );
}
