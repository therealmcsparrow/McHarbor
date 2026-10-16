// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';
import { useImageHistoryRevealed } from '@resources/hooks/useImages';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatBytes, timeAgo } from '@resources/utils/format';
import type { ImageHistoryItem } from '@core/types/docker';

function formatInstruction(command: string) {
  return command.replace(/^\/bin\/sh -c #\(nop\)\s+/, '').replace(/^\/bin\/sh -c\s+/, 'RUN ');
}

type ImageHistoryCardProps = {
  imageId: string;
  history: ImageHistoryItem[];
};

export function ImageHistoryCard({ imageId, history: stored }: ImageHistoryCardProps) {
  const { t } = useTranslation('images');
  const [revealed, setRevealed] = useState(false);
  const revealQuery = useImageHistoryRevealed(imageId, revealed);
  const hasRedacted = stored.some((layer) => layer.CreatedBy.includes(ENV_REDACTION_MARKER));
  const layers = revealed && revealQuery.data ? revealQuery.data : stored;

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('detail.layerHistory')}
      </h3>
      {(hasRedacted || revealed) && (
        <div className="mb-3 flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">{t('detail.historyWithheld')}</p>
          <Button
            variant="ghost"
            size="sm"
            permission={PERM.IMAGES_MANAGE}
            disabled={revealed && revealQuery.isFetching}
            onClick={() => setRevealed((value) => !value)}
          >
            {revealed ? <IconEyeOff className="h-3.5 w-3.5" /> : <IconEye className="h-3.5 w-3.5" />}
            {revealed ? t('detail.hideValues') : t('detail.revealValues')}
          </Button>
        </div>
      )}
      {layers.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('detail.noLayerHistory')}</p>
      ) : (
        <Table className="min-w-full">
          <TableHeader>
            <TableRow>
              <TableHead>{t('detail.history.instruction')}</TableHead>
              <TableHead>{t('detail.history.size')}</TableHead>
              <TableHead>{t('detail.history.created')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {layers.map((layer) => (
              <TableRow key={`${layer.Id}-${layer.Created}`}>
                <TableCell className="max-w-xl truncate font-mono text-xs">
                  {formatInstruction(layer.CreatedBy)}
                </TableCell>
                <TableCell className="text-xs whitespace-nowrap text-muted-foreground">
                  {formatBytes(Math.max(layer.Size, 0))}
                </TableCell>
                <TableCell className="text-xs whitespace-nowrap text-muted-foreground">
                  {timeAgo(new Date(layer.Created * 1000).toISOString())}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
