// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { type Catalog, useUpdateCatalog } from '../hooks/useCatalogs';
import { PERM } from '@core/auth/permissionKeys';

interface EditCatalogDialogProps {
  /** Row being edited, or null when the dialog is closed. */
  catalog: Catalog | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface EditCatalogFormProps {
  catalog: Catalog;
  onDone: () => void;
}

/**
 * The fields, split out so `key={catalog.id}` on the call site remounts them
 * for a different row. That is React's own reset-state-with-a-key pattern,
 * and it is used here in place of a useEffect that copies props into state:
 * an effect keyed on the catalog object would re-seed on every background
 * refetch of the catalogs query and wipe whatever the operator had typed.
 */
function EditCatalogForm({ catalog, onDone }: EditCatalogFormProps) {
  const { t } = useTranslation('settings');
  const update = useUpdateCatalog();
  const [catalogName, setCatalogName] = useState(catalog.name);
  const [url, setUrl] = useState(catalog.url);

  const trimmedName = catalogName.trim();
  const trimmedUrl = url.trim();
  const isDirty = trimmedName !== catalog.name || trimmedUrl !== catalog.url;
  const canSubmit = !!trimmedName && !!trimmedUrl && isDirty && !update.isPending;

  const submit = () => {
    if (!canSubmit) return;
    update.mutate({ id: catalog.id, patch: { name: trimmedName, url: trimmedUrl } }, { onSuccess: onDone });
  };

  return (
    <>
      <div className="space-y-3">
        <div className="space-y-1">
          <Label htmlFor="edit-catalog-name">{t('catalogs.field.name')}</Label>
          <Input
            id="edit-catalog-name"
            value={catalogName}
            onChange={(e) => setCatalogName(e.target.value)}
            placeholder={t('catalogs.field.namePlaceholder')}
            autoComplete="off"
          />
        </div>

        <div className="space-y-1">
          <Label htmlFor="edit-catalog-url">{t('catalogs.field.url')}</Label>
          <Input
            id="edit-catalog-url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={t('catalogs.field.urlPlaceholder')}
            autoComplete="off"
            inputMode="url"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={onDone}>
          {t('common.cancel')}
        </Button>
        <Button permission={PERM.SETTINGS_MANAGE} onClick={submit} disabled={!canSubmit}>
          {update.isPending ? t('common.saving') : t('catalogs.editSubmit')}
        </Button>
      </DialogFooter>
    </>
  );
}

/**
 * EditCatalogDialog edits the name and URL of a single catalog row.
 *
 * Kind is deliberately absent: `UpdateRequest` on the backend carries only
 * name, url and enabled, so a Kind control here would be one the API
 * silently discards. Enabled stays on the row's Switch, where it already is.
 *
 * Built-in rows never reach this dialog — CatalogsTab disables the trigger —
 * because SeedDefaults re-canonicalizes their name and URL on every boot,
 * which would quietly undo any edit at the next restart.
 */
export function EditCatalogDialog({ catalog, open: isOpen, onOpenChange }: EditCatalogDialogProps) {
  const { t } = useTranslation('settings');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('catalogs.editTitle')}</DialogTitle>
          <DialogDescription>{t('catalogs.editDescription')}</DialogDescription>
        </DialogHeader>

        {catalog && <EditCatalogForm key={catalog.id} catalog={catalog} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
