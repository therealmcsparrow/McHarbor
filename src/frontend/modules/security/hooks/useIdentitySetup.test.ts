// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { applicablePrefill, parseGroupList, schemaKeys } from './useIdentitySetup';

describe('parseGroupList', () => {
  it('splits on newlines, commas and semicolons, trims and de-duplicates in order', () => {
    expect(parseGroupList(' ops\nengineering, ops;;  \nqa ')).toEqual(['ops', 'engineering', 'qa']);
  });
});

describe('applicablePrefill', () => {
  it('keeps only keys the plugin form declares, in either key casing', () => {
    expect(schemaKeys([{ key: 'metadata_url' }, { Key: 'entity_id' }, { label: 'x' }, null])).toEqual(
      new Set(['metadata_url', 'entity_id']),
    );
    expect(applicablePrefill({ metadata_url: 'https://idp/md', issuer_url: 'x' }, [{ key: 'metadata_url' }])).toEqual({
      metadata_url: 'https://idp/md',
    });
    expect(applicablePrefill({ metadata_url: 'https://idp/md' }, undefined)).toEqual({});
  });
});
