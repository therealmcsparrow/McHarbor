// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useLogForwarderHistory } from '../hooks/useLogForwarders';

type LogForwarderHistoryProps = {
  forwarderId: string;
  /** False for an unavailable adapter or one with no history support. */
  supported: boolean;
};

/**
 * Delivery history for one forwarder, behind a toggle.
 *
 * It answers "is this sink actually receiving events", which an operator
 * wants occasionally rather than on every render — so the query only runs
 * once the section is open.
 */
export function LogForwarderHistory({ forwarderId, supported }: LogForwarderHistoryProps) {
  const { t } = useTranslation('settings');
  const [isOpen, setOpen] = useState(false);
  const historyQuery = useLogForwarderHistory(forwarderId, supported && isOpen);

  if (!supported) return null;

  return (
    <div className="mt-3 border-t border-border pt-3">
      <Button variant="ghost" size="sm" onClick={() => setOpen((current) => !current)} aria-expanded={isOpen}>
        <IconChevronDown className={`size-4 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
        {t('logging.history')}
      </Button>

      {isOpen && (
        <div className="mt-2 space-y-2">
          {historyQuery.isLoading && <Spinner size="sm" />}
          {(historyQuery.data ?? []).map((item) => (
            <div
              key={item.id}
              className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs"
            >
              <span className="font-mono text-muted-foreground">{item.createdAt}</span>
              <Badge variant={item.status === 'success' ? 'success' : 'destructive'}>{item.status}</Badge>
              <span className="font-mono text-muted-foreground">
                {item.protocol.toUpperCase()}
                {item.statusCode ? ` ${item.statusCode}` : ''}
              </span>
            </div>
          ))}
          {historyQuery.data?.length === 0 && <p className="text-xs text-muted-foreground">{t('logging.noHistory')}</p>}
        </div>
      )}
    </div>
  );
}
