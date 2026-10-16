// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// UMD entry for storage-samba.
//
// Host loads this via <PluginBundleLoader
//   src="/api/plugins/loader/bundle?slug=storage-samba" />.
// The bundle is built by vite-plugin.config.ts to
//   storage-samba.umd.cjs
// with the global window.McharborPlugin_storage_samba.
//
// Exposes a single function: mount(el, ctx) -> { cleanup, update }.
//   - ctx.kind tells the bundle which page to render
//     ('config_form' or 'setup_guide').
//   - The bundle is purely additive: the host owns the default
//     renderer (via ConfigFieldDecl), and this mount only fires
//     when the host is showing the plugin-supplied override.

import React from 'react';
import { type Root } from 'react-dom/client';

import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { guardedRoot } from './mountGuard';

export interface PluginMountCtx {
  kind: 'config_form' | 'setup_guide';
  // The current row's decrypted cfg map (host-supplied; the bundle
  // never sees the encryption key). Mutating it does NOT persist —
  // the host owns persistence and calls cfg.onChange().
  cfg: Record<string, string>;
  // The destination LocationType. This plugin owns only "samba";
  // keeping it in ctx means future multi-type plugins (fork) can
  // branch without an SDK bump.
  locationType: string;
  // Live updates from form edits. The host wires onChange to its
  // own state; calling onChange does not auto-save.
  onChange?: (cfg: Record<string, string>) => void;
  // The user-facing translations bundle for namespace
  // 'storage-samba'. The host may pre-load this if it knows the
  // plugin's i18n_namespaces ahead of time.
  i18n?: Record<string, string>;
}

export interface PluginMountHandle {
  cleanup: () => void;
  update: (next: PluginMountCtx) => void;
}

const rootRef: { current: Root | null } = { current: null };
const elRef: { current: HTMLElement | null } = { current: null };
const ctxRef: { current: PluginMountCtx | null } = { current: null };

function render() {
  if (!rootRef.current || !ctxRef.current) return;
  const ctx = ctxRef.current;
  const Page = ctx.kind === 'setup_guide' ? SetupGuide : ConfigForm;
  rootRef.current.render(
    <React.StrictMode>
      <Page ctx={ctx} />
    </React.StrictMode>,
  );
}

export function mount(el: HTMLElement, ctx: PluginMountCtx): PluginMountHandle {
  elRef.current = el;
  ctxRef.current = ctx;
  rootRef.current = guardedRoot(el);
  render();
  return {
    cleanup: () => {
      try {
        rootRef.current?.unmount();
      } catch {
        // Root may have already unmounted (StrictMode double-call).
      }
      rootRef.current = null;
      elRef.current = null;
      ctxRef.current = null;
    },
    update: (next) => {
      ctxRef.current = next;
      render();
    },
  };
}

export default mount;
