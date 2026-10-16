// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { IconFileExport } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { useExportRunbook, useExportRunbookDocument } from '@resources/hooks/useRunbookExport';
import { useRunbookDocumentFormats } from '../hooks/useRunbookDocumentFormats';

type RunbookExportMenuProps = {
  id: string;
  name: string;
};

const itemClass =
  'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-foreground outline-none data-[highlighted]:bg-muted data-[disabled]:opacity-50';

export function RunbookExportMenu({ id, name: itemName }: RunbookExportMenuProps) {
  const { t } = useTranslation('runbooks');
  const exportJson = useExportRunbook();
  const exportDocument = useExportRunbookDocument();
  const formats = useRunbookDocumentFormats();

  if (!formats.available) {
    return (
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('list.exportJson')}
        disabled={exportJson.isPending}
        onClick={() => exportJson.mutate({ id, name: itemName })}
      >
        <IconFileExport className="size-4" aria-hidden="true" />
      </Button>
    );
  }

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('list.export')}
          disabled={exportJson.isPending || exportDocument.isPending}
        >
          <IconFileExport className="size-4" aria-hidden="true" />
        </Button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="z-50 min-w-48 rounded-md border border-border bg-popover p-1 shadow-md"
        >
          <DropdownMenu.Item className={itemClass} onSelect={() => exportJson.mutate({ id, name: itemName })}>
            {t('list.exportJson')}
          </DropdownMenu.Item>
          <DropdownMenu.Item
            className={itemClass}
            onSelect={() => exportDocument.mutate({ id, name: itemName, format: 'pdf' })}
          >
            {t('list.exportPdf')}
          </DropdownMenu.Item>
          <DropdownMenu.Item
            className={itemClass}
            onSelect={() => exportDocument.mutate({ id, name: itemName, format: 'docx' })}
          >
            {t('list.exportDocx')}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
