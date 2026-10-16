// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArchive, IconEye } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatBytes } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import {
  BUNDLE_CATEGORIES,
  type BundleCategory,
  useBundlePreview,
  useGenerateBundle,
} from '@resources/hooks/useSupport';
import { SupportBundlePreviewDialog } from './SupportBundlePreviewDialog';

export function SupportBundleForm({ busy }: { busy: boolean }) {
  const { t } = useTranslation('support');
  const [selected, setSelected] = useState<BundleCategory[]>([...BUNDLE_CATEGORIES]);
  const [previewOpen, setPreviewOpen] = useState(false);
  const preview = useBundlePreview();
  const generate = useGenerateBundle();
  const none = selected.length === 0;

  const toggle = (c: BundleCategory, on: boolean) => {
    setSelected((cur) => BUNDLE_CATEGORIES.filter((k) => (k === c ? on : cur.includes(k))));
    preview.reset();
  };

  const openPreview = () => {
    setPreviewOpen(true);
    preview.mutate(selected);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('bundle.categoriesTitle')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('bundle.categoriesHint')}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        <ul className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {BUNDLE_CATEGORIES.map((c) => (
            <li key={c} className="flex items-start gap-3 rounded-md border border-border p-3">
              <Checkbox
                id={`bundle-${c}`}
                checked={selected.includes(c)}
                onCheckedChange={(v) => toggle(c, v === true)}
                className="mt-0.5"
              />
              <Label htmlFor={`bundle-${c}`} className="space-y-0.5">
                <span className="block">{t(`bundle.category.${c}.title`)}</span>
                <span className="block text-xs font-normal text-muted-foreground">
                  {t(`bundle.category.${c}.description`)}
                </span>
              </Label>
            </li>
          ))}
        </ul>
        {none && <Alert variant="warning">{t('bundle.noneSelected')}</Alert>}
        {preview.data && !none && (
          <p className="text-sm text-muted-foreground">
            {t('bundle.estimate', { size: formatBytes(preview.data.totalBytes) })}
          </p>
        )}
        {generate.error && <Alert variant="destructive">{generate.error.message}</Alert>}
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={openPreview} disabled={none} permission={PERM.SUPPORT_VIEW}>
            <IconEye className="size-4" />
            {t('bundle.preview')}
          </Button>
          <Button
            onClick={() => generate.mutate(selected)}
            disabled={none || busy || generate.isPending}
            permission={PERM.SUPPORT_MANAGE}
          >
            {generate.isPending ? <Spinner size="sm" /> : <IconArchive className="size-4" />}
            {t('bundle.generate')}
          </Button>
        </div>
      </CardContent>
      <SupportBundlePreviewDialog open={previewOpen} onOpenChange={setPreviewOpen} preview={preview} />
    </Card>
  );
}
