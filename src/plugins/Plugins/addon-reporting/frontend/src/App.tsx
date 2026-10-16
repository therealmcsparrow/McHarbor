// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { StudioPage } from './pages/StudioPage';
import { LibraryPage } from './pages/LibraryPage';
import { SchedulesPage } from './pages/SchedulesPage';
import { ViewerPage } from './pages/ViewerPage';
import { SettingsPage } from './pages/SettingsPage';
import type { PluginNavContext } from './index';

// The plugin's own router.
//
// The host mounts this bundle once for the whole `/plugins/addon-reporting/*`
// subtree and tells it where it is — on mount, and again on every
// navigation below that path. That is what lets one plugin back three
// sidebar entries: the alternative would be reaching into the host's
// history, which would fight the host router rather than follow it.
//
// Layout contract: the route is declared `scroll: 'data'`, so `AppLayout`
// has already wrapped us in `<PageShell scroll="data" className="p-6">`.
// Padding and the outer frame are therefore not ours to add — a second
// `PageLayout` here would double the `p-6` and open a second scroller,
// which is exactly what `PageShell`'s single-scroller assertion warns
// about. Titles go through `PageHeader`, which portals into the topbar
// slot like every host page, rather than being drawn in the page body.

type View = 'studio' | 'library' | 'schedules' | 'settings';

type Props = {
  initial: PluginNavContext;
  subscribe: (listener: (ctx: PluginNavContext) => void) => () => void;
};

export function App({ initial, subscribe }: Props) {
  const { t } = useTranslation('reporting');
  const [ctx, setCtx] = useState<PluginNavContext>(initial);
  const [openReport, setOpenReport] = useState<string | null>(null);

  useEffect(() => subscribe(setCtx), [subscribe]);

  const view = viewOf(ctx.splat);

  // Opening a report from any page shows the viewer; going back returns
  // to whichever page the sidebar says we are on.
  const openReportById = useCallback((id: string) => setOpenReport(id), []);
  const closeReport = useCallback(() => setOpenReport(null), []);

  // A sidebar click always leaves the viewer, so the entry the user
  // pressed is the thing they end up looking at.
  const [seenSplat, setSeenSplat] = useState(ctx.splat);
  if (seenSplat !== ctx.splat) {
    setSeenSplat(ctx.splat);
    setOpenReport(null);
  }

  // Both table pages render their own `PageHeader` through `TablePage`;
  // drawing a second one here would fight them for the topbar slot. The
  // viewer claims the topbar and page-head band itself, through
  // `HeaderBreadcrumb` and its own `PageHeadIdentity`/`PageHeadActions`, so
  // the generic header below is skipped while a report is open too.
  const tableView = !openReport && (view === 'library' || view === 'schedules');

  return (
    <div className="flex h-full min-h-0 flex-col">
      {!tableView && !openReport && (
        <PageHeader title={t(`page.${view}.title`)} description={t(`page.${view}.subtitle`)} />
      )}

      {openReport ? (
        // The viewer is a detail page: it pins its own hero and scrolls
        // only the document, so it must not be wrapped in a second scroller.
        <ViewerPage id={openReport} onBack={closeReport} />
      ) : tableView ? (
        view === 'schedules' ? (
          <SchedulesPage />
        ) : (
          <LibraryPage onOpen={openReportById} />
        )
      ) : (
        // A `data` shell does not scroll itself — the inner region does.
        // Prose-shaped views inside this plugin are that region.
        <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
          {view === 'settings' ? <SettingsPage /> : <StudioPage onOpen={openReportById} />}
        </div>
      )}
    </div>
  );
}

function viewOf(splat: string): View {
  const head = splat.split('/')[0] ?? '';
  if (head === 'library') return 'library';
  if (head === 'schedules') return 'schedules';
  if (head === 'settings') return 'settings';
  return 'studio';
}
