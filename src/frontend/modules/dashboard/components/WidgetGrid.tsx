// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Suspense, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ResponsiveGridLayout, useContainerWidth, type Layout, type LayoutItem } from 'react-grid-layout';
import 'react-grid-layout/css/styles.css';
import { useWidgetDefinitionMap } from '../widgets/registry';
import { useDashboardLayoutStore } from '../stores/dashboard-layout';
import { WidgetShell } from './WidgetShell';

export function WidgetGrid() {
  const { t } = useTranslation('dashboard');
  const { widgets, layouts, editMode, removeWidget, resizeWidget, updateLayouts } = useDashboardLayoutStore();
  const widgetMap = useWidgetDefinitionMap();
  const { width, containerRef, mounted } = useContainerWidth({ initialWidth: 1200 });

  const responsiveLayouts = useMemo(
    () => ({
      lg: layouts.lg as readonly LayoutItem[],
      md: layouts.md as readonly LayoutItem[],
      sm: layouts.sm as readonly LayoutItem[],
    }),
    [layouts]
  );

  const renderedWidgets = useMemo(
    () =>
      widgets.map((w) => {
        const def = widgetMap[w.typeId];
        if (!def) return null;
        const Component = def.component;
        const item = layouts.lg.find((l) => l.i === w.id);
        const colSpan = item?.w ?? def.defaultSize.w;
        const gridW = item?.w ?? def.defaultSize.w;
        const gridH = item?.h ?? def.defaultSize.h;

        return (
          <div key={w.id}>
            <WidgetShell
              editMode={editMode}
              title={t(def.labelKey)}
              width={gridW}
              height={gridH}
              onRemove={() => removeWidget(w.id)}
              onResize={(newW, newH) => resizeWidget(w.id, newW, newH)}
            >
              <Suspense
                fallback={
                  <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
                    {t('loading')}
                  </div>
                }
              >
                <Component colSpan={colSpan} typeId={w.typeId} />
              </Suspense>
            </WidgetShell>
          </div>
        );
      }),
    [widgets, layouts.lg, editMode, removeWidget, resizeWidget, widgetMap, t]
  );

  return (
    <div ref={containerRef} className="relative">
      {/* Grid background overlay — visible only in edit mode */}
      {editMode && (
        <div
          className="pointer-events-none absolute inset-0 grid grid-cols-12 gap-4"
          aria-hidden="true"
        >
          {Array.from({ length: 12 }, (_, i) => (
            <div
              key={`grid-column-${i + 1}`}
              className="rounded-lg border border-dashed border-muted-foreground/[0.08] bg-muted-foreground/[0.02]"
            />
          ))}
        </div>
      )}

      {mounted && (
        <ResponsiveGridLayout
          width={width}
          layouts={responsiveLayouts}
          breakpoints={{ lg: 1200, md: 768, sm: 480 }}
          cols={{ lg: 12, md: 8, sm: 4 }}
          rowHeight={100}
          margin={[16, 16]}
          dragConfig={{ enabled: editMode, handle: '.widget-drag-handle' }}
          resizeConfig={{ enabled: false }}
          onLayoutChange={(_current: Layout, allLayouts) => {
            const all = allLayouts as Record<string, Layout>;
            updateLayouts({
              lg: [...(all['lg'] ?? layouts.lg)],
              md: [...(all['md'] ?? layouts.md)],
              sm: [...(all['sm'] ?? layouts.sm)],
            });
          }}
        >
          {renderedWidgets}
        </ResponsiveGridLayout>
      )}
    </div>
  );
}
