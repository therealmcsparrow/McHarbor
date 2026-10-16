// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// identity-google plugin frontend entry. Mirrors comm-signal:
// exports a `mount(el, ctx)` returning {cleanup, update} so the
// host's PluginFormOutlet can push value snapshots back without
// remounting, plus named `ConfigForm` and `SetupGuide` exports for
// callers that want to render them directly.

import { type Root } from 'react-dom/client';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { identityGoogleResources } from './i18n';
import { guardedRoot } from './mountGuard';

// Host context passed to mount(). Loose-typed so the bundle can be
// compiled without the host's TS project. Field names mirror
// PluginFormOutlet's contract: value, onChange, onError,
// readOnlyFields, kind, t.
export type PluginHostContext = {
  value?: Record<string, string>;
  onChange?: (values: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  readOnlyFields?: string[];
  kind?: 'config_form' | 'setup_guide';
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

let activeRoot: Root | null = null;
let currentCtx: PluginHostContext = {};

// Initialise this bundle's i18next instance with the EN resources
// shipped at src/i18n so useTranslation() calls inside ConfigForm
// stop emitting NO_I18NEXT_INSTANCE at module-eval time. The host's
// window.i18next instance (if present) is preferred per render; this
// init only matters for standalone bundle use + first mount.
let i18nInited = false;
function ensureI18n(): void {
  if (i18nInited) return;
  i18nInited = true;
  try {
    void i18next.use(initReactI18next).init({
      lng: typeof navigator !== 'undefined' ? navigator.language : 'en',
      fallbackLng: 'en',
      resources: identityGoogleResources,
      interpolation: { escapeValue: false },
    });
  } catch {
    // Best effort — react-i18next falls back to the raw key when
    // no instance is available, so a failed init never breaks the
    // form (it just shows literal "fields.tenant_id" etc.).
  }
}

// mount() returns {cleanup, update} so the host can push new value
// snapshots back into the bundle without remounting. Mirrors
// comm-signal's pattern.
export function mount(
  el: HTMLElement,
  ctx: PluginHostContext = {},
): {
  cleanup: () => void;
  update: (patch: Partial<PluginHostContext>) => void;
} {
  ensureI18n();
  currentCtx = ctx;
  activeRoot = guardedRoot(el);
  render();
  return {
    cleanup: () => {
      activeRoot?.unmount();
      activeRoot = null;
      currentCtx = {};
    },
    update: (patch) => {
      currentCtx = { ...currentCtx, ...patch };
      render();
    },
  };
}

function render(): void {
  if (!activeRoot) return;
  const { value, onChange, onError, kind, t } = currentCtx;
  if (kind === 'setup_guide') {
    activeRoot.render(<SetupGuide />);
    return;
  }
  // Fully controlled — ConfigForm is rendered with the host's value
  // directly. No local useState, so the host's source of truth flows
  // through unchanged.
  activeRoot.render(<ConfigForm value={value ?? {}} onChange={onChange ?? (() => {})} onError={onError} t={t} />);
}

export { ConfigForm };
export { SetupGuide };
export default { mount };
