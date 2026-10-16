// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatBytes } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { useDatabaseCleanup, useRemovePreviousDatabase } from '../hooks/useDatabase';

/**
 * Delete the SQLite file a completed migration left behind.
 *
 * Only ever offered for SQLite, and only after McHarbor has moved off it. A
 * PostgreSQL or MySQL database left behind belongs to whoever administers
 * that server — dropping it is their decision, with their tooling, and a
 * button here would be McHarbor destroying data on a machine it does not own.
 *
 * The card renders nothing when there is nothing to delete, so it does not
 * sit on the page as a permanent invitation. The one exception is `in_use`:
 * an operator who came looking for this button deserves to be told why it is
 * not there rather than left wondering.
 */
export function DatabaseCleanupCard() {
  const { t } = useTranslation('database');
  const cleanup = useDatabaseCleanup();
  const remove = useRemovePreviousDatabase();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const currentStatus = cleanup.data;
  if (!currentStatus) return null;
  if (!currentStatus.available && currentStatus.reason !== 'in_use') return null;

  if (!currentStatus.available) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('cleanup.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <TypographyText size="sm" tone="muted">
            {t('cleanup.inUse')}
          </TypographyText>
        </CardContent>
      </Card>
    );
  }

  const confirmAction = () => {
    remove.mutate(undefined, {
      onSuccess: (result) => {
        setConfirmOpen(false);
        toast.success(t('cleanup.removed', { count: result.removed.length }));
      },
      onSettled: () => setConfirmOpen(false),
    });
  };

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('cleanup.title')}</CardTitle>
          <TypographyText size="sm" tone="muted">
            {t('cleanup.description')}
          </TypographyText>
        </CardHeader>
        <CardContent>
          <Stack gap={4}>
            <Box className="rounded-lg border border-border bg-muted/20 p-3">
              <Stack gap={2}>
                {(currentStatus.files ?? []).map((file) => (
                  <Flex key={file.path} align="center" justify="between" gap={3}>
                    <TypographyText size="sm" className="truncate font-mono">
                      {file.path}
                    </TypographyText>
                    <TypographyText size="xs" tone="muted" className="shrink-0">
                      {formatBytes(file.bytes)}
                    </TypographyText>
                  </Flex>
                ))}
              </Stack>
            </Box>

            <Flex align="center" justify="between" gap={3} wrap>
              <TypographyText size="sm" tone="muted">
                {t('cleanup.reclaim', { size: formatBytes(currentStatus.totalBytes) })}
              </TypographyText>
              <Button
                variant="destructive"
                permission={PERM.DATABASE_MANAGE}
                disabled={remove.isPending}
                onClick={() => setConfirmOpen(true)}
              >
                <IconTrash className="size-4" aria-hidden />
                {t('cleanup.delete')}
              </Button>
            </Flex>

            <TypographyText size="sm" tone="muted">
              {t('cleanup.checkFirst')}
            </TypographyText>
          </Stack>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('cleanup.confirmTitle')}
        description={t('cleanup.confirmBody', {
          path: currentStatus.path ?? '',
          count: currentStatus.files?.length ?? 0,
          size: formatBytes(currentStatus.totalBytes),
        })}
        confirmLabel={t('cleanup.delete')}
        variant="destructive"
        loading={remove.isPending}
        onConfirm={confirmAction}
      />
    </>
  );
}
