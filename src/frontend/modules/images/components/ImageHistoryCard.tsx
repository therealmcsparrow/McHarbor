// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatBytes, timeAgo } from '@resources/utils/format';
import type { ImageHistoryItem } from '@core/types/docker';

function formatInstruction(command: string) {
  return command.replace(/^\/bin\/sh -c #\(nop\)\s+/, '').replace(/^\/bin\/sh -c\s+/, 'RUN ');
}

type ImageHistoryCardProps = {
  history: ImageHistoryItem[];
};

export function ImageHistoryCard({ history }: ImageHistoryCardProps) {
  const { t } = useTranslation('images');

  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t('detail.layerHistory')}
      </h3>
      {history.length === 0 ? (
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
            {history.map((layer) => (
              <TableRow key={`${layer.Id}-${layer.Created}`}>
                <TableCell className="max-w-xl truncate font-mono text-xs">
                  {formatInstruction(layer.CreatedBy)}
                </TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                  {layer.Size > 0 ? formatBytes(layer.Size) : '0 B'}
                </TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
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
