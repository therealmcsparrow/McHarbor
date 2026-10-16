// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The plugin's own router.
//
// The host mounts this bundle once for the whole `/plugins/addon-kubernetes/*`
// subtree and tells it where it is — on mount, and again on every navigation
// below that path. That is what lets one plugin back seven sidebar entries.
//
// Layout contract: the plugin route is declared `scroll: 'data'`, so
// `AppLayout` has already wrapped us in `<PageShell scroll="data"
// className="p-6">`. Padding and the outer frame are therefore not ours to
// add — a `PageLayout` here would double the `p-6` and open a second scroller,
// which is exactly what `PageShell`'s single-scroller assertion warns about.
// (The core page carried that bug until the restyle immediately before this
// migration; it is not being carried across.)

import { useEffect, useState } from 'react';
import { setSelectedCluster } from './cluster-store';
import { PluginNavProvider } from './nav-context';
import { ClustersPage } from './pages/ClustersPage';
import { KubernetesPage } from './pages/KubernetesPage';
import type { PluginNavContext } from './index';

type Props = {
  initial: PluginNavContext;
  subscribe: (listener: (ctx: PluginNavContext) => void) => () => void;
};

export function App({ initial, subscribe }: Props) {
  const [ctx, setCtx] = useState<PluginNavContext>(initial);

  useEffect(() => subscribe(setCtx), [subscribe]);

  // A palette hit found on another cluster arrives as ?cluster=<id>; selecting it is
  // what makes the page read that cluster rather than the one last chosen here.
  useEffect(() => {
    const cluster = new URLSearchParams(ctx.search).get('cluster');
    if (cluster) setSelectedCluster(cluster);
  }, [ctx.search]);

  // `clusters` is the connection admin page and shares nothing with the
  // resource browser — different data, different permission, its own table.
  // Everything else is the browser, which reads its tab out of the splat.
  const isClusterAdmin = ctx.splat.split('/')[0] === 'clusters';

  return <PluginNavProvider value={ctx}>{isClusterAdmin ? <ClustersPage /> : <KubernetesPage />}</PluginNavProvider>;
}
