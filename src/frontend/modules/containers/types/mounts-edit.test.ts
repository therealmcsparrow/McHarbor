// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { mountsToPayload, parseMounts, type MountSource } from './mounts-edit';

const inspect: MountSource = {
  Mounts: [
    { Type: 'bind', Source: '/srv/ha', Destination: '/config', Mode: '', RW: true, Propagation: 'rprivate' },
    {
      Type: 'volume',
      Name: 'ha-media',
      Source: '/var/lib/docker/volumes/ha-media/_data',
      Destination: '/media',
      Mode: 'z',
      RW: false,
      Propagation: '',
    },
    {
      Type: 'volume',
      Name: 'mcharbor-secrets-abc',
      Source: '/x',
      Destination: '/run/secrets',
      Mode: '',
      RW: false,
      Propagation: '',
    },
  ],
  HostConfig: { Tmpfs: { '/run/cache': 'size=64m' } },
};

describe('configuration editor mounts', () => {
  it('lists every mount but the secret-files one, including --tmpfs mounts Docker keeps elsewhere', () => {
    expect(parseMounts(inspect)).toEqual([
      { type: 'bind', source: '/srv/ha', target: '/config', readOnly: false, propagation: 'rprivate' },
      { type: 'volume', source: 'ha-media', target: '/media', readOnly: true, propagation: '' },
      { type: 'tmpfs', source: '', target: '/run/cache', readOnly: false, propagation: '' },
    ]);
  });

  it('never sends the secret-files mount, and sends tmpfs without a source', () => {
    expect(
      mountsToPayload([
        { type: 'tmpfs', source: 'ignored', target: '/run/cache', readOnly: false, propagation: '' },
        { type: 'bind', source: ' /srv/x ', target: '/run/secrets', readOnly: true, propagation: '' },
        { type: 'bind', source: '/srv/ha', target: '/config', readOnly: true, propagation: 'rslave' },
      ]),
    ).toEqual([
      { type: 'tmpfs', source: undefined, target: '/run/cache', readOnly: undefined, propagation: undefined },
      { type: 'bind', source: '/srv/ha', target: '/config', readOnly: true, propagation: 'rslave' },
    ]);
  });
});
