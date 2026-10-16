// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Plugin entry: UMD bundle consumed by the host's
// PluginPageHost. The host's loader calls mount(el, ctx) and
// stores the returned cleanup + update handles for later
// teardown / hot-context updates.
//
// Mount lifecycle:
//   1. host attaches el to the plugin's section of the page
//   2. mount(el, ctx) creates a React root and renders <App />
//   3. host later calls cleanup() on unmount
//   4. host calls update({ cfg, errors }) when the host's
//      config context changes (e.g. user edits a field) so the
//      plugin can re-validate without remounting
//
// Cfg shape: see backend/main.go ftpConfigSchema(). Fields:
//   server, port, auth_method, username, password,
//   tls_skip_verify, client_cert_path, client_key_path,
//   path, connect_timeout.

import { type Root } from 'react-dom/client';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import ConfigForm from './ConfigForm';
import SetupGuide from './SetupGuide';
import { guardedRoot } from './mountGuard';

export interface PluginContext {
  // Current config values from the host. The plugin reads this
  // on mount and on update().
  cfg: Record<string, string>;
  // Field-level errors returned by the host from the last
  // validation. Keyed by ConfigFieldDecl.Key.
  errors?: Record<string, string>;
  // 'config' | 'guide' — the host's RendererSlot name.
  mode?: 'config' | 'guide';
  // Optional: change handler. The plugin's ConfigForm calls
  // this on every keystroke so the host can react (e.g. enable
  // the Save button).
  onChange?: (cfg: Record<string, string>) => void;
}

export interface MountedPlugin {
  cleanup: () => void;
  update: (ctx: PluginContext) => void;
}

function App({ ctx, onChange }: { ctx: PluginContext; onChange?: (cfg: Record<string, string>) => void }) {
  // The plugin uses the 'storage-ftp' i18n namespace declared
  // in plugin.yaml i18n_namespaces. The host bundles the
  // plugin's locale files into the runtime i18next instance.
  const { t } = useTranslation('storage-ftp');
  const [mode, setMode] = useState<'config' | 'guide'>(ctx.mode ?? 'config');

  // Sync mode changes when the host switches tabs.
  const [seenMode, setSeenMode] = useState(ctx.mode);
  if (seenMode !== ctx.mode) {
    setSeenMode(ctx.mode);
    if (ctx.mode && ctx.mode !== mode) setMode(ctx.mode);
  }

  if (mode === 'guide') {
    return <SetupGuide cfg={ctx.cfg} />;
  }
  return (
    <ConfigForm
      cfg={ctx.cfg}
      errors={ctx.errors}
      onChange={onChange}
      heading={t('config.heading')}
      description={t('config.description')}
    />
  );
}

export function mount(el: HTMLElement, ctx: PluginContext): MountedPlugin {
  let currentCtx: PluginContext = ctx;
  let onChange: ((cfg: Record<string, string>) => void) | undefined = ctx.onChange;

  const root: Root = guardedRoot(el);
  const render = () => {
    root.render(<App ctx={currentCtx} onChange={onChange} />);
  };
  render();

  return {
    cleanup: () => {
      root.unmount();
    },
    update: (next: PluginContext) => {
      currentCtx = next;
      if (next.onChange !== onChange) {
        onChange = next.onChange;
      }
      render();
    },
  };
}
