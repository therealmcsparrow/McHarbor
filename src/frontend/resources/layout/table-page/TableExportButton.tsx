// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import type { RowData, Table } from '@resources/components/table-features';
import { Button } from '@resources/components/ui/Button';
import { Popover, PopoverTrigger, PopoverContent } from '@resources/components/ui/Popover';
import { tableToCsv, tableToJson, downloadTableExport } from '@resources/utils/table-export';

type TableExportButtonProps<T extends RowData> = {
  table: Table<T>;
  filename: string;
  /** When set, the button calls this instead of exporting client-side. */
  server?: () => void;
  /** True when the list is server-paged, so a client export covers one page. */
  serverPaged: boolean;
};

export function TableExportButton<T extends RowData>({
  table,
  filename,
  server,
  serverPaged,
}: TableExportButtonProps<T>) {
  const { t } = useTranslation('common');

  // A server endpoint is the only path that can reach beyond the loaded page,
  // so when one exists it replaces the client-side menu entirely.
  if (server) {
    return (
      <Button variant="outline" size="icon-sm" aria-label={t('table.export')} onClick={server}>
        <IconDownload className="size-4" />
      </Button>
    );
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="icon-sm" aria-label={t('table.export')}>
          <IconDownload className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-52 p-2">
        <div className="flex flex-col gap-1">
          <Button
            variant="ghost"
            size="sm"
            className="justify-start"
            onClick={() => downloadTableExport(tableToCsv(table), filename, 'csv')}
          >
            {t('table.exportCsv')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="justify-start"
            onClick={() => downloadTableExport(tableToJson(table), filename, 'json')}
          >
            {t('table.exportJson')}
          </Button>
          {serverPaged && (
            <span className="px-2 pt-1 text-xs text-muted-foreground">{t('table.exportCurrentPage')}</span>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
