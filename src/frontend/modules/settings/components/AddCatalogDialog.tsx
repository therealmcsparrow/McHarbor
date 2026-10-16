// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
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
import { Select } from '@resources/components/ui/Select';
import { visibleCatalogKinds, type VisibleCatalogKind, useCatalogs, useCreateCatalog } from '../hooks/useCatalogs';
import { PERM } from '@core/auth/permissionKeys';

interface AddCatalogDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultKind?: VisibleCatalogKind;
}

export function AddCatalogDialog({ open: isOpen, onOpenChange, defaultKind }: AddCatalogDialogProps) {
  const { t } = useTranslation('settings');
  const create = useCreateCatalog();
  const catalogs = useCatalogs();
  const [catalogName, setCatalogName] = useState('');
  const [url, setUrl] = useState('');
  const [kind, setKind] = useState<VisibleCatalogKind>(defaultKind ?? 'docker_app');

  const reset = () => {
    setCatalogName('');
    setUrl('');
    setKind(defaultKind ?? 'docker_app');
  };

  const handleClose = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const submit = () => {
    if (!catalogName.trim() || !url.trim()) return;
    create.mutate({ name: catalogName.trim(), url: url.trim(), kind }, { onSuccess: () => handleClose(false) });
  };

  const kindOptions = useMemo(
    () =>
      visibleCatalogKinds(catalogs.data).map((k) => ({
        value: k,
        label: t(`catalogs.kind.${k}`),
      })),
    [catalogs.data, t],
  );

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('catalogs.addTitle')}</DialogTitle>
          <DialogDescription>{t('catalogs.addDescription')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="catalog-name">{t('catalogs.field.name')}</Label>
            <Input
              id="catalog-name"
              value={catalogName}
              onChange={(e) => setCatalogName(e.target.value)}
              placeholder={t('catalogs.field.namePlaceholder')}
              autoComplete="off"
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="catalog-url">{t('catalogs.field.url')}</Label>
            <Input
              id="catalog-url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder={t('catalogs.field.urlPlaceholder')}
              autoComplete="off"
              inputMode="url"
            />
          </div>

          <div className="space-y-1">
            <Label>{t('catalogs.field.kind')}</Label>
            <Select
              value={kind}
              onChange={(v) => setKind(v as VisibleCatalogKind)}
              options={kindOptions}
              ariaLabel={t('catalogs.field.kind')}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            onClick={submit}
            disabled={!catalogName.trim() || !url.trim() || create.isPending}
          >
            {create.isPending ? t('common.saving') : t('catalogs.addSubmit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
