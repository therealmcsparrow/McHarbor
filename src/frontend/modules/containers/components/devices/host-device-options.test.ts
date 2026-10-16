// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { freeContainerPath } from './host-device-options';

describe('freeContainerPath', () => {
  it('keeps a path nobody else uses', () => {
    expect(freeContainerPath('/dev/ttyUSB0', ['/dev/ttyACM0'])).toBe('/dev/ttyUSB0');
  });

  it('increments the trailing number past every taken path', () => {
    expect(freeContainerPath('/dev/ttyUSB0', ['/dev/ttyUSB0', ' /dev/ttyUSB1 ', '/dev/ttyUSB3'])).toBe('/dev/ttyUSB2');
  });

  it('appends a number to a path without one', () => {
    expect(freeContainerPath('/dev/zigbee', ['/dev/zigbee', '/dev/zigbee1'])).toBe('/dev/zigbee2');
  });
});
