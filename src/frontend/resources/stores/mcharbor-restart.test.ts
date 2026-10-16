// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { assertRestartScheduled } from './mcharbor-restart';

const NATIVE = 'McHarbor is not running under systemd';

describe('assertRestartScheduled', () => {
  it('replaces the Docker wording with the native explanation when nothing supervises McHarbor', () => {
    expect(() =>
      assertRestartScheduled(
        {
          success: false,
          code: 'err.system.restart_not_local',
          error: 'Restart is only supported when McHarbor runs as a local Docker container',
          data: { reason: 'not_supervised' },
        },
        NATIVE,
      ),
    ).toThrow(NATIVE);
  });

  it('keeps the server message for any other refusal, and passes a scheduled restart through', () => {
    expect(() =>
      assertRestartScheduled(
        { success: false, code: 'err.system.restart_not_scheduled', error: 'Restart was not scheduled' },
        NATIVE,
      ),
    ).toThrow('Restart was not scheduled');
    const data = {
      message: 'ok',
      code: 'msg.system.restart_scheduled',
      result: { containerId: '', containerName: 'mcharbor.service', scheduledAt: '' },
    };
    expect(assertRestartScheduled({ success: true, data }, NATIVE)).toEqual(data);
  });
});
