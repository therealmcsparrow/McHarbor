// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// UMD entry for the snapshot-truenas plugin. The host's plugin
// loader fetches this bundle and mounts it into the snapshot
// configuration dialog.
//
// Two modes are exposed:
//   - kind="config_form" → renders ConfigForm
//   - kind="setup_guide" → renders SetupGuide

import React from 'react';
import { type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { guardedRoot } from './mountGuard';

export interface PluginProps {
  kind: 'config_form' | 'setup_guide';
  driver?: string;
  formData?: Record<string, unknown>;
  onChange?: (data: Record<string, unknown>) => void;
  locale?: string;
}

let root: Root | null = null;

export function mount(el: HTMLElement, props: PluginProps) {
  root = guardedRoot(el);
  root.render(<PluginHost {...props} />);
}

export function unmount() {
  root?.unmount();
  root = null;
}

function PluginHost(props: PluginProps) {
  if (props.kind === 'setup_guide') {
    return <SetupGuide driver={props.driver ?? 'truenas'} locale={props.locale ?? 'en'} />;
  }
  return (
    <ConfigForm
      driver={props.driver ?? 'truenas'}
      formData={props.formData ?? {}}
      onChange={props.onChange ?? (() => {})}
    />
  );
}
