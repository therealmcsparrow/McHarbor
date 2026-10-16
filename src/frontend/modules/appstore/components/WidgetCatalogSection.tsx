// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { PERM } from '@core/auth/permissionKeys';
import {
  useInstallCatalogWidget,
  useWidgetCatalogEntries,
  type CatalogWidget,
} from '@resources/hooks/useWidgetCatalog';
import { PluginIcon } from './plugin-icons';
import { RemoveDownloadedWidgetDialog } from './RemoveDownloadedWidgetDialog';

function CatalogRow({ widget, onRemove }: { widget: CatalogWidget; onRemove: (w: CatalogWidget) => void }) {
  const { t } = useTranslation('common');
  const install = useInstallCatalogWidget();
  const upToDate = widget.installed && widget.installedVersion === widget.version;

  return (
    <li className="flex flex-col gap-2 rounded-lg border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <PluginIcon icon={widget.icon} className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium text-foreground">{widget.label}</p>
          <p className="text-xs text-muted-foreground">{widget.description}</p>
          <div className="mt-1 flex flex-wrap gap-1">
            <Badge variant="info">{t('store.widgetVersion', { version: widget.version })}</Badge>
            {widget.installed ? <Badge variant="success">{t('store.installed')}</Badge> : null}
            {widget.signed ? null : <Badge variant="warning">{t('store.widgetUnsigned')}</Badge>}
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          {upToDate ? null : (
            <Button
              size="sm"
              permission={PERM.DASHBOARD_WIDGETS_INSTALL}
              disabled={!widget.signed || install.isPending}
              onClick={() => install.mutate(widget.key)}
            >
              {widget.installed ? t('store.updateAction') : t('appStore.install')}
            </Button>
          )}
          {widget.installed ? (
            <Button
              size="sm"
              variant="outline"
              permission={PERM.DASHBOARD_WIDGETS_INSTALL}
              onClick={() => onRemove(widget)}
            >
              {t('actions.remove')}
            </Button>
          ) : null}
        </div>
      </div>
      {install.isError ? (
        <ErrorState
          variant="inline"
          title={t('store.widgetInstallFailed', { name: widget.label })}
          error={install.error}
        />
      ) : null}
    </li>
  );
}

export function WidgetCatalogSection() {
  const { t } = useTranslation('common');
  const { data } = useWidgetCatalogEntries();
  const [removing, setRemoving] = useState<CatalogWidget | null>(null);

  if (!data || data.length === 0) return null;

  return (
    <section className="flex flex-col gap-3" aria-labelledby="widget-catalog-title">
      <div>
        <h2 id="widget-catalog-title" className="text-sm font-semibold text-foreground">
          {t('store.widgetCatalogTitle')}
        </h2>
        <p className="text-xs text-muted-foreground">{t('store.widgetCatalogDescription')}</p>
      </div>
      <ul className="grid gap-2 md:grid-cols-2">
        {data.map((widget) => (
          <CatalogRow key={widget.key} widget={widget} onRemove={setRemoving} />
        ))}
      </ul>
      {removing ? (
        <RemoveDownloadedWidgetDialog
          widgetKey={removing.key}
          widgetName={removing.label}
          open
          onOpenChange={(next) => {
            if (!next) setRemoving(null);
          }}
        />
      ) : null}
    </section>
  );
}
