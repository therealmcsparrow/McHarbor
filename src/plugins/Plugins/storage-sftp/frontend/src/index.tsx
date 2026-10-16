// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Plugin frontend entry point for storage-sftp.
//
// Mount contract (see `.claude/skills/plugin/SKILL.md`):
//   - the host's PluginPageHost calls window.McharborPlugin_storage_sftp.mount(el, ctx)
//     when a storage_location of location_type="sftp" is being edited.
//   - the host provides React (window.McharborSharedReact) and
//     react-i18next (window.McharborSharedI18n) globally; we
//     pull them off the global rather than importing locally
//     so we share one copy with the host bundle.
//
// Two mounts, one element:
//   - config_form slot: renders the per-field form with
//     auth_method conditional rendering.
//   - setup_guide slot: renders the operator-facing walkthrough.
//     The host chooses which component to mount via the
//     `slot` argument.

import * as React from 'react';
import { type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { guardedRoot } from './mountGuard';

type MountContext = {
  // Pre-translated i18n instance from the host. Plugins share
  // one i18next bundle; we attach our namespace lazily.
  i18n: typeof import('i18next').default;
  // Slot name from the host — "config_form" or "setup_guide".
  slot: string;
  // Initial config the host loaded from the destination row.
  config: Record<string, string>;
  // Host-callback for changes — invoked on every keystroke /
  // dropdown change. Debounced in the host.
  onChange: (next: Record<string, string>) => void;
  // Optional: surface inline errors (host renders them under
  // the field). null when nothing to report.
  errors: Record<string, string> | null;
};

type MountFn = (el: HTMLElement, ctx: MountContext) => void;
type UnmountFn = () => void;

let activeRoot: Root | null = null;
let activeEl: HTMLElement | null = null;

const PluginRoot: React.FC<{ ctx: MountContext }> = ({ ctx }) => {
  return ctx.slot === 'setup_guide' ? (
    <SetupGuide config={ctx.config} />
  ) : (
    <ConfigForm config={ctx.config} onChange={ctx.onChange} errors={ctx.errors} />
  );
};

const mount: MountFn = (el, ctx) => {
  if (activeRoot) {
    activeRoot.unmount();
    activeRoot = null;
  }
  if (activeEl && activeEl !== el) {
    activeEl.innerHTML = '';
  }
  activeEl = el;
  activeRoot = guardedRoot(el);
  activeRoot.render(<PluginRoot ctx={ctx} />);
};

const unmount: UnmountFn = () => {
  if (activeRoot) {
    activeRoot.unmount();
    activeRoot = null;
  }
  if (activeEl) {
    activeEl.innerHTML = '';
    activeEl = null;
  }
};

// Top-level exports, because the UMD global IS the exports namespace: a named
// export matching `lib.name` would put mount one level too deep.
export { mount, unmount };
export default { mount, unmount };
