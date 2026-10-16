// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * storage-nfs frontend UMD entry.
 *
 * The host loads this bundle via
 * window.McharborPlugin_storage_nfs and calls
 * mount(el, ctx) once per slot. The host passes
 * ctx.kind ("config_form" or "setup_guide") so the plugin
 * can branch on which view to render.
 *
 * The plugin owns a single LocationType ("nfs"); there is no
 * cfg["location_type"] branch in the form.
 *
 * Note on i18n: react-i18next is bundled inline here without an
 * i18next instance, so components render through the host's `t`
 * from ctx (namespace = the plugin slug).
 */

import { type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { guardedRoot } from './mountGuard';

let root: Root | null = null;
let currentCtx: MountCtx | null = null;

export type MountCtx = {
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  kind?: 'config_form' | 'setup_guide';
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

function render() {
  if (!root || !currentCtx) return;
  const { kind, value, onChange, onError, t } = currentCtx;
  root.render(
    kind === 'setup_guide' ? (
      <SetupGuide value={value} t={t} />
    ) : (
      <ConfigForm value={value} onChange={onChange} onError={onError} t={t} />
    ),
  );
}

// mount() returns { cleanup, update } so the host can push
// new `value` snapshots back into the bundle without
// remounting. The legacy API (cleanup-only function) still
// works — the host checks the return shape.
export function mount(
  el: HTMLElement,
  ctx: MountCtx,
): {
  cleanup: () => void;
  update: (patch: Partial<MountCtx>) => void;
} {
  root = guardedRoot(el);
  currentCtx = ctx;
  render();
  return {
    cleanup: () => {
      root?.unmount();
      root = null;
      currentCtx = null;
    },
    update: (patch) => {
      if (!currentCtx) return;
      currentCtx = { ...currentCtx, ...patch };
      render();
    },
  };
}

export { ConfigForm, SetupGuide };
export default { mount };
