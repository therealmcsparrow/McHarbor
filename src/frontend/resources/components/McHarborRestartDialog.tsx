// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconAlertTriangle, IconRefresh } from '@tabler/icons-react';
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
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';
import { Caption, Text as UiText } from '@resources/components/ui/Typography';
import { useMcHarborRestartStore } from '@resources/stores/mcharbor-restart';

export function McHarborRestartDialog() {
  const { t } = useTranslation('common');
  const { open: isOpen, phase, progress, reset } = useMcHarborRestartStore();
  const failed = phase === 'error';
  const errorDescription = t('restartProgress.phases.error.description');

  return (
    <Dialog open={isOpen} onOpenChange={(nextOpen) => !nextOpen && failed && reset()}>
      <DialogContent
        className="max-w-md p-0"
        hideCloseButton
        onEscapeKeyDown={(evt) => !failed && evt.preventDefault()}
        onPointerDownOutside={(evt) => evt.preventDefault()}
      >
        <DialogHeader className="space-y-1">
          <DialogTitle>{t('restartProgress.title')}</DialogTitle>
          <DialogDescription>{t('restartProgress.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <Stack gap={5}>
            <Flex direction="col" align="center" gap={3} className="py-3 text-center">
              <Box className="flex size-16 items-center justify-center bg-primary/10 text-primary">
                {failed ? (
                  <IconAlertTriangle className="size-7 text-destructive" />
                ) : phase === 'ready' ? (
                  <IconRefresh className="size-7 animate-spin" />
                ) : (
                  <Spinner size="lg" />
                )}
              </Box>
              <Box role={failed ? 'alert' : 'status'} aria-live={failed ? 'assertive' : 'polite'} aria-atomic="true">
                <UiText weight="semibold">{t(`restartProgress.phases.${phase}.title`)}</UiText>
                <UiText size="sm" tone="muted" className="mt-1">
                  {failed ? errorDescription : t(`restartProgress.phases.${phase}.description`)}
                </UiText>
              </Box>
            </Flex>
            <Box>
              <Flex justify="between" align="center" className="mb-2">
                <Caption tone="muted">{t('restartProgress.progressLabel')}</Caption>
                <Caption tone="muted" className="tabular-nums">
                  {progress}%
                </Caption>
              </Flex>
              <ProgressBar value={progress} aria-hidden="true" />
              <progress
                className="sr-only"
                value={progress}
                max={100}
                aria-label={t('restartProgress.progressLabel')}
              />
            </Box>
            {!failed && (
              <UiText size="xs" tone="muted" className="text-center">
                {t('restartProgress.automaticReload')}
              </UiText>
            )}
          </Stack>
        </DialogBody>
        {failed && (
          <DialogFooter>
            <Button
              variant="outline"
              size="lg"
              className="focus-visible:ring-2 focus-visible:ring-ring"
              onClick={reset}
            >
              {t('actions.close')}
            </Button>
            <Button
              size="lg"
              className="focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => window.location.reload()}
            >
              <IconRefresh className="size-4" />
              {t('restartProgress.reloadNow')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
