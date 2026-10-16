// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import type { Mode, Translate } from './useZammadValidation';
import { guardedRoot } from './mountGuard';

type MountCtx = {
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  kind?: 'config_form' | 'setup_guide';
  mode?: Mode;
  t?: Translate;
};

const identity: Translate = (key) => key;

export function mount(el: HTMLElement, initial: MountCtx) {
  let root: Root | null = guardedRoot(el);
  let ctx = initial;
  const render = () => {
    const t = ctx.t ?? identity;
    root?.render(
      ctx.kind === 'setup_guide' ? (
        <SetupGuide t={t} />
      ) : (
        <ConfigForm value={ctx.value} onChange={ctx.onChange} onError={ctx.onError} mode={ctx.mode} t={t} />
      ),
    );
  };
  render();
  return {
    cleanup: () => {
      root?.unmount();
      root = null;
    },
    update: (patch: Partial<MountCtx>) => {
      ctx = { ...ctx, ...patch };
      render();
    },
  };
}

export { ConfigForm, SetupGuide };
export default { mount };
