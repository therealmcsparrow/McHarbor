// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { i18n } from '../../../test/i18n';
import { reportRecreateOutcome } from './recreateOutcome';

const toasts = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), details: vi.fn() }));
vi.mock('@resources/utils/toast', () => ({
  appToast: { success: toasts.success, warning: toasts.warning },
  toastErrorDetails: toasts.details,
}));

const t = (key: string, options?: Record<string, unknown>) => i18n.t(key, { ns: 'networks', ...options });

describe('reportRecreateOutcome', () => {
  beforeEach(() => vi.clearAllMocks());

  it('reports plain success when every container kept its address', () => {
    reportRecreateOutcome([{ container: 'c1', name: 'web', previousIpv4: '192.168.1.50', addressKept: true }], t);
    expect(toasts.success).toHaveBeenCalledWith('Network updated');
    expect(toasts.warning).not.toHaveBeenCalled();
  });

  it('names each container that came back on a new address, with the address it lost', () => {
    reportRecreateOutcome(
      [
        { container: 'c1', name: 'web', previousIpv4: '192.168.1.50', addressKept: false },
        { container: 'c2', name: 'db', previousIpv4: '192.168.1.51', addressKept: true },
      ],
      t,
    );
    expect(toasts.warning).toHaveBeenCalledTimes(1);
    expect(toasts.warning.mock.calls[0]?.[0]).toContain('web (192.168.1.50)');
    expect(toasts.warning.mock.calls[0]?.[0]).not.toContain('db');
    expect(toasts.success).not.toHaveBeenCalled();
  });

  it('reports a failed reconnect with the daemon message as detail', () => {
    reportRecreateOutcome([{ container: 'c1', name: 'web', addressKept: false, error: 'Address already in use' }], t);
    expect(toasts.details).toHaveBeenCalledWith(expect.objectContaining({ detail: 'web: Address already in use' }));
    expect(toasts.warning).not.toHaveBeenCalled();
  });
});
