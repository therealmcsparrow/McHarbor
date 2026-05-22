// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLayoutDashboard, IconPlus, IconRotate } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { useDashboardLayoutStore } from '../stores/dashboard-layout';
import { useWidgetSync } from '../hooks/useWidgetSync';

const WidgetGrid = lazy(() => import('../components/WidgetGrid').then((m) => ({ default: m.WidgetGrid })));
const WidgetPicker = lazy(() => import('../components/WidgetPicker').then((m) => ({ default: m.WidgetPicker })));

export default function DashboardPage() {
  const { t } = useTranslation('dashboard');
  const { editMode, setEditMode, resetToDefault } = useDashboardLayoutStore();
  const [pickerOpen, setPickerOpen] = useState(false);
  useWidgetSync();

  return (
    <div className="space-y-4">
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <div className="flex items-center gap-2">
            {editMode && (
              <>
                <Button variant="outline" size="sm" onClick={() => setPickerOpen(true)}>
                  <IconPlus className="mr-1.5 h-4 w-4" />
                  {t('addWidget')}
                </Button>
                <Button variant="outline" size="sm" onClick={resetToDefault}>
                  <IconRotate className="mr-1.5 h-4 w-4" />
                  {t('reset')}
                </Button>
              </>
            )}
            <Button
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

      <Suspense fallback={<div className="h-64 rounded-lg border border-border bg-card/50" />}>
        <WidgetGrid />
      </Suspense>
      {pickerOpen ? (
        <Suspense fallback={null}>
          <WidgetPicker open={pickerOpen} onOpenChange={setPickerOpen} />
        </Suspense>
      ) : null}
    </div>
  );
}
