// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createRoot, type Root } from 'react-dom/client';

type HostWindow = Window & {
  McharborHost?: { i18n?: { t: (key: string, options?: Record<string, unknown>) => string } };
};

// A plugin root is invisible to the host's error boundaries, so a render crash
// would otherwise leave this mount point blank with no explanation.
function MountFailed() {
  const t = (window as HostWindow).McharborHost?.i18n?.t;
  const message = t ? t('pluginMount.failed', { ns: 'common' }) : 'pluginMount.failed';
  return <p className="p-4 text-sm text-muted-foreground">{message}</p>;
}

export function guardedRoot(el: HTMLElement): Root {
  const root: Root = createRoot(el, {
    onUncaughtError: (error) => {
      console.error(error);
      queueMicrotask(() => root.render(<MountFailed />));
    },
  });
  return root;
}
