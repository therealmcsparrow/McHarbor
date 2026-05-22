// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileText, IconSearch } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { useStackLogs } from '../hooks/useStacks';
import type { StackInfo } from '../hooks/useStacks';

type StackLogsDialogProps = {
  stack: StackInfo | null;
  onClose: () => void;
};

export function StackLogsDialog({ stack, onClose }: StackLogsDialogProps) {
  const { t } = useTranslation('stacks');
  const { data: logs, isLoading } = useStackLogs(stack?.name ?? null);
  const [activeService, setActiveService] = useState<string | null>(null);
  const [filter, setFilter] = useState('');

  useEffect(() => {
    setActiveService(null);
    setFilter('');
  }, [stack?.name]);

  const serviceNames = logs ? Object.keys(logs).sort() : [];
  const selectedService = activeService ?? serviceNames[0] ?? null;
  const rawLogs = selectedService && logs ? logs[selectedService] ?? '' : '';
  const logLines = rawLogs.split('\n').filter(Boolean);
  const filteredLines = filter
    ? logLines.filter((line) => line.toLowerCase().includes(filter.toLowerCase()))
    : logLines;

  return (
    <Dialog open={stack !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-4xl p-0">
        <DialogHeader className="px-4 py-3">
          <DialogTitle className="flex items-center gap-2 text-sm">
            <IconFileText className="h-4 w-4 text-cyan-400" />
            {t('logs.title', { name: stack?.name })}
          </DialogTitle>
          <DialogDescription className="sr-only">{t('logs.srDescription')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="p-0">
          {isLoading ? (
            <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
              {t('logs.loading')}
            </div>
          ) : (
            <div className="flex flex-col gap-3 px-4 pb-4">
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-1 overflow-x-auto">
                {serviceNames.map((name) => (
                  <Button
                    key={name}
                    variant={selectedService === name ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setActiveService(name)}
                  >
                    {name}
                  </Button>
                ))}
              </div>
              <div className="relative flex-1">
                <IconSearch className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t('logs.filterPlaceholder')}
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  className="pl-9"
                />
              </div>
            </div>

            <div className="overflow-hidden rounded-lg border border-border bg-[#0a0a0a]">
              <pre
                className="max-h-[500px] min-h-[300px] overflow-auto p-4 font-mono text-xs leading-5 text-zinc-300"
              >
                {filteredLines.map((line, i) => (
                  <div key={`${selectedService}-${i}`} className="hover:bg-foreground/5">
                    {line}
                  </div>
                ))}
                {filteredLines.length === 0 && (
                  <span className="text-muted-foreground">
                    {filter ? t('logs.noMatchingLines') : t('logs.noLogsAvailable')}
                  </span>
                )}
              </pre>
            </div>
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
