// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * comm-teams frontend UMD entry.
 *
 * The host loads this bundle via window.McharborPlugin_comm_teams
 * and calls mount(el, ctx) once per slot. ctx.kind tells us which
 * view to render.
 *
 * This bundle owns a single ChannelType ("teams"); there is no
 * cfg["channel_type"] branch in the form.
 */

import { type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { guardedRoot } from './mountGuard';

let root: Root | null = null;
let currentCtx: MountCtx | null = null;

type MountCtx = {
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
      <SetupGuide t={t} />
    ) : (
      <ConfigForm value={value} onChange={onChange} onError={onError} t={t} />
    ),
  );
}

// mount() returns { cleanup, update } so the host can push new
// `value` snapshots back into the bundle without remounting. The
// old API (cleanup-only function) still works — the host checks
// the return shape and degrades gracefully.
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
