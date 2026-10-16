// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The plugin's own router.
//
// The host mounts this bundle once for the whole `/plugins/addon-swarm/*`
// subtree and tells it where it is — on mount, and again on every navigation
// below that path. That is what lets one plugin back four sidebar entries and
// ten pages.
//
// It replaces ten entries in the host's `core/router.tsx`. A `switch` reads
// worse than a route table on paper, but it is doing less: there are no
// params to match (the plugin reads its id out of the splat), no lazy
// boundaries to declare (the bundle is one file by the time it gets here), and
// no scroll handles (the host declared one for the whole subtree).
//
// Layout contract: the plugin route is declared `scroll: 'data'`, so
// `AppLayout` has already wrapped us in `<PageShell scroll="data"
// className="p-6">`. Padding and the outer frame are therefore not ours to
// add — a second `p-6` here would double the gutter and a `PageLayout` would
// open a second scroller, which is exactly what `PageShell`'s single-scroller
// assertion warns about.

import { useEffect, useState } from 'react';
import { PluginNavProvider } from './nav-context';
import { isDetailSplat, swarmTabFromSplat } from './nav';
import SwarmOverviewPage from './pages/SwarmOverviewPage';
import SwarmServicesPage from './pages/SwarmServicesPage';
import SwarmServiceDetailPage from './pages/SwarmServiceDetailPage';
import SwarmTasksPage from './pages/SwarmTasksPage';
import SwarmNodesPage from './pages/SwarmNodesPage';
import SwarmNodeDetailPage from './pages/SwarmNodeDetailPage';
import SwarmSecretsPage from './pages/SwarmSecretsPage';
import SwarmConfigsPage from './pages/SwarmConfigsPage';
import SwarmStacksPage from './pages/SwarmStacksPage';
import SwarmNetworksPage from './pages/SwarmNetworksPage';
import type { PluginNavContext } from './index';

type Props = {
  initial: PluginNavContext;
  subscribe: (listener: (ctx: PluginNavContext) => void) => () => void;
};

export function App({ initial, subscribe }: Props) {
  const [ctx, setCtx] = useState<PluginNavContext>(initial);

  useEffect(() => subscribe(setCtx), [subscribe]);

  return <PluginNavProvider value={ctx}>{render(ctx.splat)}</PluginNavProvider>;
}

function render(splat: string) {
  const tab = swarmTabFromSplat(splat);
  // Only two tabs have a detail page below them. Everything else ignores the
  // extra segment rather than 404ing on it: a link with a stray trailing path
  // should still land on the list it names.
  const detail = isDetailSplat(splat);

  switch (tab) {
    case 'services':
      return detail ? <SwarmServiceDetailPage /> : <SwarmServicesPage />;
    case 'nodes':
      return detail ? <SwarmNodeDetailPage /> : <SwarmNodesPage />;
    case 'tasks':
      return <SwarmTasksPage />;
    case 'secrets':
      return <SwarmSecretsPage />;
    case 'configs':
      return <SwarmConfigsPage />;
    case 'stacks':
      return <SwarmStacksPage />;
    case 'networks':
      return <SwarmNetworksPage />;
    case 'overview':
    default:
      return <SwarmOverviewPage />;
  }
}
