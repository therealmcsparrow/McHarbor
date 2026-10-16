// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReportType } from '../types';

type Translate = (key: string, options?: Record<string, unknown>) => string;
type Exists = (key: string, options?: Record<string, unknown>) => boolean;

export function providerNamespace(type: Pick<ReportType, 'provider' | 'label_ns' | 'label_key'>): string | null {
  if (!type.provider) return null;
  const key = type.label_key ?? '';
  const colon = key.indexOf(':');
  if (colon > 0) return key.slice(0, colon);
  return type.label_ns || type.provider;
}

export function optionLabel(
  t: Translate,
  exists: Exists,
  type: Pick<ReportType, 'provider' | 'label_ns' | 'label_key'>,
  key: string,
): string {
  const ns = providerNamespace(type);
  if (ns && exists(`reports.options.${key}`, { ns })) {
    return t(`reports.options.${key}`, { ns });
  }
  return t(`option.${key}`, { ns: 'reporting' });
}
