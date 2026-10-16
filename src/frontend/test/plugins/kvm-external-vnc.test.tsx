// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it, vi } from 'vitest';

// The menu reads the live noVNC session only for paste and screenshot; this test
// needs neither, and the real module would pull noVNC in.
vi.mock('../../../plugins/Plugins/addon-kvm/frontend/src/console-sessions', () => ({
  consoleRFB: () => null,
}));

import { ConsoleMenu } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/VMConsoleMenu';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { renderWithProviders } from '../render';

describe('addon-kvm external VNC client', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  });

  it('names the exact CLI command for this guest and the local address to connect to', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ConsoleMenu vmId="vm-123" vmName="srv1" environmentId="env-9" />);

    await user.click(screen.getByRole('button', { name: 'More console actions' }));

    expect(await screen.findByText('External VNC client')).toBeInTheDocument();
    expect(screen.getByText('mcharbor kvm vm console vm-123 --console vnc --env env-9')).toBeInTheDocument();
    expect(screen.getByText(/connect the VNC client to localhost:5900/)).toBeInTheDocument();
  });
});
