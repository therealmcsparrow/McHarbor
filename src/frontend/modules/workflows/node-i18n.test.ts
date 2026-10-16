// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { buildNodeTranslations } from '@nodes/i18n-loader';

describe('buildNodeTranslations', () => {
  it('nests each node bundle under its node key, the shape tn(`${key}.label`) reads', async () => {
    const en = await buildNodeTranslations('en');
    expect(en).toMatchObject({
      parse: { label: 'Parse' },
      junction: { label: 'Junction' },
    });
  });

  it('keeps nodes from overwriting each other', async () => {
    const en = (await buildNodeTranslations('en')) as Record<string, { label?: string }>;
    expect(en.label).toBeUndefined();
    expect(en.parse?.label).not.toBe(en.junction?.label);
  });

  it('loads the requested language only', async () => {
    const de = (await buildNodeTranslations('de')) as Record<string, { label?: string }>;
    expect(de.parse?.label).toBe('Parsen');
  });
});
