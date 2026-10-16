// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { itemCovered } from './run-item-coverage';

describe('itemCovered', () => {
  // The homelab01 report: data/ restored, every file in it shown "Not included".
  it('counts the files and folders inside a restored folder as restored', () => {
    const included = ['folder:/data:data'];
    expect(itemCovered('file:/data:data/auth.db', included)).toBe(true);
    expect(itemCovered('folder:/data:data/.cache/thumbnails/', included)).toBe(true);
    expect(itemCovered('folder:/data:data/', included)).toBe(true);
    expect(itemCovered('file:/data:database.db', included)).toBe(false);
    expect(itemCovered('file:/other:data/auth.db', included)).toBe(false);
  });

  it('counts everything in a whole mount, the filesystem layer and the config unit', () => {
    expect(itemCovered('file:/data:x/y', ['mount:/data'])).toBe(true);
    expect(itemCovered('file:/config:x', ['mount:/data'])).toBe(false);
    expect(itemCovered('folder:filesystem:etc', ['filesystem'])).toBe(true);
    expect(itemCovered('config:Config.Env', ['config'])).toBe(true);
    expect(itemCovered('image', ['config'])).toBe(false);
  });
});
