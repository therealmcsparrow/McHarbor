// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import {
  IconBookmark,
  IconBookmarkPlus,
  IconPencil,
  IconRefresh,
  IconStar,
  IconStarOff,
  IconTrash,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { cn } from '@resources/utils/cn';
import { SaveFilterDialog } from './SaveFilterDialog';
import type { SavedFiltersController } from './useTableSavedFilters';

const itemClass =
  'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-foreground outline-none data-[highlighted]:bg-muted data-[disabled]:opacity-50';

export function TableSavedFiltersMenu({ controller }: { controller: SavedFiltersController }) {
  const { t } = useTranslation('common');
  const [dialog, setDialog] = useState<{ mode: 'create' | 'rename'; id?: string; name: string } | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const active = controller.filters.find((entry) => entry.id === controller.activeId);

  return (
    <>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <Button variant="outline" size="sm" disabled={!controller.available}>
            <IconBookmark className="size-4" />
            <span className="max-w-40 truncate">{active?.name ?? t('filters.savedFilters')}</span>
            {controller.modified && (
              <span className="size-1.5 rounded-full bg-primary" aria-label={t('tableViews.modified')} />
            )}
          </Button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            align="end"
            sideOffset={4}
            className="z-50 min-w-64 rounded-md border border-border bg-popover p-1 shadow-md"
          >
            {controller.filters.length === 0 && (
              <p className="px-2 py-1.5 text-sm text-muted-foreground">{t('tableViews.noSavedFilters')}</p>
            )}
            {controller.filters.map((filter) => (
              <DropdownMenu.Sub key={filter.id}>
                <DropdownMenu.SubTrigger
                  className={cn(itemClass, filter.id === controller.activeId && 'font-medium')}
                  onClick={() => controller.apply(filter.id)}
                >
                  {filter.isDefault ? <IconStar className="size-4 text-primary" /> : <span className="size-4" />}
                  <span className="min-w-0 flex-1 truncate">{filter.name}</span>
                </DropdownMenu.SubTrigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.SubContent className="z-50 min-w-52 rounded-md border border-border bg-popover p-1 shadow-md">
                    <DropdownMenu.Item className={itemClass} onSelect={() => controller.apply(filter.id)}>
                      <IconBookmark className="size-4" />
                      {t('tableViews.apply')}
                    </DropdownMenu.Item>
                    <DropdownMenu.Item className={itemClass} onSelect={() => controller.update(filter.id)}>
                      <IconRefresh className="size-4" />
                      {t('tableViews.updateCurrent')}
                    </DropdownMenu.Item>
                    <DropdownMenu.Item
                      className={itemClass}
                      onSelect={() => setDialog({ mode: 'rename', id: filter.id, name: filter.name })}
                    >
                      <IconPencil className="size-4" />
                      {t('tableViews.rename')}
                    </DropdownMenu.Item>
                    <DropdownMenu.Item
                      className={itemClass}
                      onSelect={() => controller.setDefault(filter.id, !filter.isDefault)}
                    >
                      {filter.isDefault ? <IconStarOff className="size-4" /> : <IconStar className="size-4" />}
                      {filter.isDefault ? t('tableViews.unsetDefault') : t('tableViews.setDefault')}
                    </DropdownMenu.Item>
                    <DropdownMenu.Item
                      className={cn(itemClass, 'text-destructive')}
                      onSelect={() => setDeleting(filter.id)}
                    >
                      <IconTrash className="size-4" />
                      {t('actions.delete')}
                    </DropdownMenu.Item>
                  </DropdownMenu.SubContent>
                </DropdownMenu.Portal>
              </DropdownMenu.Sub>
            ))}
            <DropdownMenu.Separator className="my-1 h-px bg-border" />
            <DropdownMenu.Item className={itemClass} onSelect={() => setDialog({ mode: 'create', name: '' })}>
              <IconBookmarkPlus className="size-4" />
              {t('tableViews.saveCurrent')}
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>

      <SaveFilterDialog
        open={dialog !== null}
        onOpenChange={(isOpen) => !isOpen && setDialog(null)}
        mode={dialog?.mode ?? 'create'}
        initialName={dialog?.name ?? ''}
        onSubmit={(itemName, isDefault) =>
          dialog?.mode === 'rename' && dialog.id
            ? controller.rename(dialog.id, itemName)
            : controller.create(itemName, isDefault)
        }
      />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(isOpen) => !isOpen && setDeleting(null)}
        title={t('tableViews.deleteTitle')}
        description={t('tableViews.deleteConfirm', {
          name: controller.filters.find((entry) => entry.id === deleting)?.name ?? '',
        })}
        onConfirm={() => {
          if (deleting) controller.remove(deleting);
          setDeleting(null);
        }}
      />
    </>
  );
}
