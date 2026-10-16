// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// TestChannelDialog — confirm, send, then show the per-step report.
//
// Unlike the storage and identity test modals, this one keeps a confirm
// step before it runs: a channel test actually delivers a message to a
// real recipient (an inbox, a Slack channel, a phone), so firing it the
// moment the button is clicked would be wrong. The three states are:
//
//   confirm  ──Send test──▶  running (spinner)  ──▶  results
//
// The results view is the same layout, status colours, and step list as
// StorageLocationTestDialog and IdentityProviderTestDialog.
//
// Steps come from src/backend/modules/communications/test.go — load,
// config, plugin, send. Step details are host-authored; the backend
// does not forward raw transport error text.

import { useState } from 'react';
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
import { useTestChannel } from '../hooks/useNotificationChannels';
import { ChannelTestReport } from './ChannelTestReport';
import { PERM } from '@core/auth/permissionKeys';

type TestChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channelId: string;
  channelName: string;
};

export function TestChannelDialog({ open: isOpen, onOpenChange, channelId, channelName }: TestChannelDialogProps) {
  const { t } = useTranslation(['settings', 'common']);
  const testChannel = useTestChannel();
  // Tracked separately from the mutation so reopening the dialog for
  // the same channel starts at the confirm step again instead of
  // showing the previous run's report.
  const [started, setStarted] = useState(false);

  const result = started ? testChannel.data : undefined;
  const pending = started && testChannel.isPending;

  function handleOpenChange(next: boolean) {
    if (!next) {
      setStarted(false);
      testChannel.reset();
    }
    onOpenChange(next);
  }

  function handleSend() {
    setStarted(true);
    testChannel.mutate(channelId);
  }

  const overall = result?.overallStatus ?? (pending ? 'running' : 'skip');

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className={started ? 'max-w-2xl' : 'max-w-md'}>
        <DialogHeader>
          <DialogTitle>{t('communications.testDialogTitle')}</DialogTitle>
          <DialogDescription>
            {pending
              ? t('communications.test.running')
              : started
                ? t('communications.test.description', { name: channelName })
                : t('communications.testDialogDescription', { name: channelName })}
          </DialogDescription>
        </DialogHeader>

        {started && (
          <DialogBody className="space-y-3">
            {pending ? (
              <div
                className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
                data-testid="channel-test-running"
              >
                <Spinner size="md" />
                <span className="text-sm text-foreground">{t('communications.test.running')}</span>
              </div>
            ) : (
              <ChannelTestReport overallStatus={overall} duration={result?.duration} steps={result?.steps ?? []} />
            )}
          </DialogBody>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={pending}>
            {started ? t('common:actions.close') : t('common:actions.cancel')}
          </Button>
          {!started && (
            <Button permission={PERM.COMMUNICATIONS_MANAGE} onClick={handleSend}>
              {t('communications.sendTest')}
            </Button>
          )}
          {started && !pending && (
            <Button variant="outline" onClick={handleSend}>
              {t('communications.test.runAgain')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
