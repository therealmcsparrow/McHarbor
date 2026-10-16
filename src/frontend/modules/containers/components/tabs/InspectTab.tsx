// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCopy, IconCheck } from '@tabler/icons-react';
import { useContainer } from '../../hooks/useContainers';
import { Spinner } from '@resources/components/ui/Spinner';
import { useFlash } from '@resources/hooks/useFlash';
import { Button } from '@resources/components/ui/Button';

type InspectTabProps = {
  containerId: string;
};

export function InspectTab({ containerId }: InspectTabProps) {
  const { t } = useTranslation('containers');
  const { data, isLoading } = useContainer(containerId);
  const { flashing: copied, flash: flashCopied } = useFlash(1500);

  const json = data ? JSON.stringify(data, null, 2) : '';

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(json);
      flashCopied();
    } catch {
      /* clipboard blocked — ignore */
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">{t('inspect.title')}</h3>
        <Button variant="ghost" size="sm" onClick={copy} disabled={!json} aria-label={t('inspect.copy')}>
          {copied ? <IconCheck className="h-4 w-4 text-success" /> : <IconCopy className="h-4 w-4" />}
          {copied ? t('inspect.copied') : t('inspect.copy')}
        </Button>
      </div>
      {isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner size="md" />
        </div>
      ) : (
        <pre className="mc-glass max-h-[60vh] overflow-auto rounded-lg p-4 font-mono text-xs text-foreground">
          {json}
        </pre>
      )}
    </div>
  );
}
