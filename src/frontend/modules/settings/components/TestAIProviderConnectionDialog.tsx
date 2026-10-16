// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useTestAIProviderConnection } from '../hooks/useAIProviderConnections';
import { AIProviderTestReport } from './AIProviderTestReport';
import { PERM } from '@core/auth/permissionKeys';

type TestAIProviderConnectionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connectionId: string;
  connectionName: string;
};

export function TestAIProviderConnectionDialog({
  open: isOpen,
  onOpenChange,
  connectionId,
  connectionName,
}: TestAIProviderConnectionDialogProps) {
  const { t } = useTranslation(['settings', 'common']);
  const testConnection = useTestAIProviderConnection();

  const { reset: resetTest } = testConnection;
  useEffect(() => {
    resetTest();
  }, [connectionId, resetTest]);

  function handleSend() {
    testConnection.mutate(connectionId);
  }

  const result = testConnection.data;
  const pending = testConnection.isPending;
  const overall = result?.overallStatus ?? (pending ? 'running' : 'skip');

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && pending) return;
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('aiProviders.test.title', { name: connectionName })}</DialogTitle>
          <DialogDescription>
            {pending ? t('aiProviders.test.running') : t('aiProviders.test.description', { name: connectionName })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {pending ? (
            <output
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="ai-test-running"
              aria-live="polite"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">{t('aiProviders.test.running')}</span>
            </output>
          ) : testConnection.isError ? (
            <ErrorState variant="card" error={testConnection.error} onRetry={handleSend} />
          ) : !result ? (
            <output className="block rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
              {t('aiProviders.test.idle', { name: connectionName })}
            </output>
          ) : (
            <AIProviderTestReport overallStatus={overall} duration={result.duration} steps={result.steps} />
          )}
        </DialogBody>
        <DialogFooter>
          {!result && !pending && !testConnection.isError && (
            <Button permission={PERM.SETTINGS_MANAGE} onClick={handleSend} disabled={pending}>
              {t('aiProviders.sendTest')}
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            {t('common:close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
