// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { usePluginForms } from '@core/plugins/loader';
import { PluginFormOutlet } from '@core/plugins/PluginFormOutlet';
import type { ReactElement } from 'react';

/**
 * ProviderFormOutlet — thin host-side helper that renders a
 * plugin-contributed (resource, kind) slot for a specific provider.
 *
 * Two patterns:
 *
 *  1. Read-only slot (e.g. setup_guide). Pass `children` as the
 *     fallback. The outlet returns either the plugin's UMD mount
 *     or the children when no plugin owns the slot.
 *
 *  2. Read-write slot (config_form). Pass `value` and `onChange`
 *     of type Record<string, string>. The plugin's ConfigForm
 *     edits the map directly; on save the host's typed save
 *     handler reads the values it cares about (e.g. webhook_url,
 *     chat_id) and ignores the rest.
 *
 * Hosts that need strict typed-data bridging can wrap the outlet
 * with a `useState<Record<string, string>>` mirror; for setup_guide
 * the prop is purely advisory.
 */

type ProviderFormOutletProps = {
  resource: 'channel' | 'email_server' | 'identity_provider' | 'storage_location' | 'ai_provider';
  kind: 'config_form' | 'setup_guide' | 'selector';
  /** Match against plugin_forms.key (e.g. "slack", "s3", "smtp"). */
  keyName: string;
  /** Required for config_form. Optional for setup_guide. */
  value?: Record<string, string>;
  onChange?: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  /** Force a specific plugin slug instead of auto-resolving. */
  pluginSlug?: string;
  /** Fallback UI when no plugin is installed. */
  children?: ReactElement | null;
};

export function ProviderFormOutlet({
  resource,
  kind,
  keyName,
  value,
  onChange,
  onError,
  pluginSlug,
  children,
}: ProviderFormOutletProps) {
  const forms = usePluginForms();
  const hasMatch = forms.some((f) => {
    if (f.resource !== resource) return false;
    if (f.kind !== kind) return false;
    if (pluginSlug && f.plugin_slug !== pluginSlug) return false;
    // Match either the keyed slot or an unkeyed wildcard.
    if (f.key && f.key !== keyName) return false;
    return true;
  });

  if (!hasMatch) return children ?? null;

  return (
    <PluginFormOutlet
      resource={resource}
      kind={kind}
      keyName={keyName}
      value={value ?? {}}
      onChange={onChange ?? (() => {})}
      onError={onError}
      pluginSlug={pluginSlug}
      i18nPrefix={`${resource}.${keyName}`}
    >
      {children}
    </PluginFormOutlet>
  );
}
