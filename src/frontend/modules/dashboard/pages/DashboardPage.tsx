// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLayoutDashboard, IconPlus, IconRotate } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { PageBody } from '@resources/layout/PageBody';
import { Button } from '@resources/components/ui/Button';
import { useEnvironmentStore } from '@resources/stores/environment';
import { EstateSummary } from '../components/EstateSummary';
import { useDashboardLayoutStore } from '../stores/dashboard-layout';
import { useWidgetSync } from '../hooks/useWidgetSync';
import { PERM } from '@core/auth/permissionKeys';
import { PluginMountBoundary } from '@resources/components/PluginMountBoundary';

const WidgetGrid = lazy(() => import('../components/WidgetGrid').then((m) => ({ default: m.WidgetGrid })));
const WidgetPicker = lazy(() =>
  import('../components/WidgetPicker').then((m) => ({
    default: m.WidgetPicker,
  })),
);

export default function DashboardPage() {
  const { t } = useTranslation('dashboard');
  const currentId = useEnvironmentStore((s) => s.currentId);
  const { editMode, setEnvironmentScope, setEditMode, resetToDefault } = useDashboardLayoutStore();
  const [pickerOpen, setPickerOpen] = useState(false);
  useWidgetSync();

  useEffect(() => {
    setEnvironmentScope(currentId);
  }, [currentId, setEnvironmentScope]);

  // The route declares `scroll: "data"`, so PageShell is a fixed frame and the
  // one scroller on this screen has to be declared here. Without it the grid
  // was simply clipped at the fold. The header stays pinned; only the widgets
  // move.
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        headingLeaf
        title={t('title')}
        description={<EstateSummary />}
        actions={
          <div className="flex flex-wrap items-center justify-end gap-2">
            {editMode && (
              <>
                <Button
                  permission={PERM.WIDGETS_MANAGE}
                  variant="outline"
                  size="sm"
                  onClick={() => setPickerOpen(true)}
                >
                  <IconPlus className="mr-1.5 h-4 w-4" />
                  {t('addWidget')}
                </Button>
                <Button permission={PERM.WIDGETS_MANAGE} variant="outline" size="sm" onClick={resetToDefault}>
                  <IconRotate className="mr-1.5 h-4 w-4" />
                  {t('reset')}
                </Button>
              </>
            )}
            <Button
              permission={PERM.WIDGETS_MANAGE}
              variant={editMode ? 'default' : 'outline'}
              size="sm"
              onClick={() => setEditMode(!editMode)}
            >
              <IconLayoutDashboard className="mr-1.5 h-4 w-4" />
              {editMode ? t('done') : t('edit')}
            </Button>
          </div>
        }
      />

      <PageBody className="pr-1">
        <PluginMountBoundary>
          <Suspense fallback={<div className="mc-glass h-64 animate-pulse rounded-lg" />}>
            <WidgetGrid />
          </Suspense>
        </PluginMountBoundary>
      </PageBody>
      {pickerOpen ? (
        <PluginMountBoundary>
          <Suspense fallback={null}>
            <WidgetPicker open={pickerOpen} onOpenChange={setPickerOpen} />
          </Suspense>
        </PluginMountBoundary>
      ) : null}
    </div>
  );
}
