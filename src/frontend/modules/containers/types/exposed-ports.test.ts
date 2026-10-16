// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { PortMappingEntry } from './edit-form';
import { exposedPortRows, releaseMappingPort, removeExposedPort } from './exposed-ports';

const map = (containerPort: string, protocol = 'tcp'): PortMappingEntry => ({
  containerPort,
  hostPort: '1' + containerPort,
  hostIp: '0.0.0.0',
  protocol,
});
const image = new Set(['8123/tcp']);

describe('exposed ports in the configuration editor', () => {
  it('locks image ports and mapped ports, and lets every other port go', () => {
    expect(exposedPortRows(['8123/tcp', '9000/tcp', '5353/udp'], [map('8080')], image)).toEqual([
      { port: '5353/udp' },
      { port: '8080/tcp', lockedBy: 'mapping' },
      { port: '8123/tcp', lockedBy: 'image' },
      { port: '9000/tcp' },
    ]);
  });

  it('removes a port the operator added, but never one the image declares', () => {
    expect(removeExposedPort(['8123/tcp', '9000/tcp'], '9000/tcp', image)).toEqual(['8123/tcp']);
    expect(removeExposedPort(['8123/tcp', '9000/tcp'], '8123/tcp', image)).toEqual(['8123/tcp', '9000/tcp']);
  });

  it('releases a removed mapping port unless the image declares it or another mapping still uses it', () => {
    expect(releaseMappingPort(['8080/tcp', '8123/tcp'], map('8080'), [], image)).toEqual(['8123/tcp']);
    expect(releaseMappingPort(['8123/tcp'], map('8123'), [], image)).toEqual(['8123/tcp']);
    expect(releaseMappingPort(['8080/tcp'], map('8080'), [map('8080')], image)).toEqual(['8080/tcp']);
    expect(releaseMappingPort(['8080/tcp'], map('8080', 'udp'), [], image)).toEqual(['8080/tcp']);
  });
});
