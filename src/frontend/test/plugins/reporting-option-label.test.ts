// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import {
  optionLabel,
  providerNamespace,
} from '../../../plugins/Plugins/addon-reporting/frontend/src/pages/option-label';

const bundles: Record<string, Record<string, string>> = {
  reporting: { 'option.include_detail': 'Include operational detail' },
  vm: { 'reports.options.onlyUnprotected': 'Only machines with no snapshot' },
  compute: { 'reports.options.onlyIssues': 'Only hosts, images and entries that need attention' },
};

const t = (key: string, options?: Record<string, unknown>) => {
  const ns = typeof options?.ns === 'string' ? options.ns : 'reporting';
  return bundles[ns]?.[key] ?? `${ns}:${key}`;
};
const exists = (key: string, options?: Record<string, unknown>) =>
  bundles[typeof options?.ns === 'string' ? options.ns : 'reporting']?.[key] !== undefined;

describe('reporting option labels', () => {
  it('takes the namespace from the label key prefix before the slug', () => {
    expect(
      providerNamespace({ provider: 'addon-kvm', label_ns: 'addon-kvm', label_key: 'vm:reports.inventory.label' }),
    ).toBe('vm');
    expect(providerNamespace({ provider: 'addon-swarm', label_ns: 'swarm', label_key: 'reporting.type.x.name' })).toBe(
      'swarm',
    );
    expect(providerNamespace({ provider: 'addon-x' })).toBe('addon-x');
    expect(providerNamespace({})).toBeNull();
  });

  it('labels a provider option from the provider bundle', () => {
    const kvm = { provider: 'addon-kvm', label_ns: 'addon-kvm', label_key: 'vm:reports.protection.label' };
    expect(optionLabel(t, exists, kvm, 'onlyUnprotected')).toBe('Only machines with no snapshot');
    const compute = { provider: 'addon-compute', label_key: 'compute:reports.hygiene.label' };
    expect(optionLabel(t, exists, compute, 'onlyIssues')).toBe('Only hosts, images and entries that need attention');
  });

  it('falls back to the shared reporting label for a common option', () => {
    const swarm = { provider: 'addon-swarm', label_ns: 'swarm', label_key: 'reporting.type.x.name' };
    expect(optionLabel(t, exists, swarm, 'include_detail')).toBe('Include operational detail');
  });

  it('labels built-in options from the reporting bundle', () => {
    expect(optionLabel(t, exists, {}, 'include_detail')).toBe('Include operational detail');
  });
});
