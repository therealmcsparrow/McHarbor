// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// UMD entry for the snapshot-synology plugin. The host's plugin
// loader fetches this bundle and mounts it into the snapshot
// configuration dialog.
//
// Two modes are exposed:
//   - kind="config_form" → renders ConfigForm
//   - kind="setup_guide" → renders SetupGuide
//
// Both modes share the same `pluginProps` shape; the host passes a
// pre-parsed form for `config_form` and a driver-key for
// `setup_guide`.

import React from 'react';
import { type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';
import { guardedRoot } from './mountGuard';

export interface PluginProps {
  kind: 'config_form' | 'setup_guide';
  // config_form:
  driver?: string;
  formData?: Record<string, unknown>;
  onChange?: (data: Record<string, unknown>) => void;
  // setup_guide:
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
    return <SetupGuide driver={props.driver ?? 'synology'} locale={props.locale ?? 'en'} />;
  }
  return (
    <ConfigForm
      driver={props.driver ?? 'synology'}
      formData={props.formData ?? {}}
      onChange={props.onChange ?? (() => {})}
    />
  );
}
