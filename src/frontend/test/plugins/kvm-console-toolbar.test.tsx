// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { TerminalFullscreenButton } from '@resources/components/TerminalFullscreen';
import { useConsoleFullscreen } from '../../../plugins/Plugins/addon-kvm/frontend/src/hooks/useConsoleFullscreen';
import { ConsoleToolbar } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/ConsoleToolbar';
import { ConsoleKeysMenu } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/ConsoleKeysMenu';
import { spiceEvents, type KeyStroke } from '../../../plugins/Plugins/addon-kvm/frontend/src/spice-keys';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';

function Toolbar({ connected, onSend }: { connected: boolean; onSend: (strokes: KeyStroke[]) => void }) {
  const { fullscreen, toggle } = useConsoleFullscreen();
  return (
    <ConsoleToolbar
      slots={{
        leading: null,
        trailing: <TerminalFullscreenButton fullscreen={fullscreen} onToggle={toggle} />,
      }}
    >
      <ConsoleKeysMenu disabled={!connected} onSend={onSend} />
    </ConsoleToolbar>
  );
}

let fullscreenElement: Element | null = null;
const requestFullscreen = vi.fn(() => {
  fullscreenElement = document.documentElement;
  document.dispatchEvent(new Event('fullscreenchange'));
  return Promise.resolve();
});
const exitFullscreen = vi.fn(() => {
  fullscreenElement = null;
  document.dispatchEvent(new Event('fullscreenchange'));
  return Promise.resolve();
});

describe('addon-kvm console toolbar', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  beforeEach(() => {
    grantPermissions(['vm.view', 'vm.console']);
    fullscreenElement = null;
    requestFullscreen.mockClear();
    exitFullscreen.mockClear();
    Object.defineProperty(document, 'fullscreenEnabled', { configurable: true, value: true });
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => fullscreenElement });
    Object.defineProperty(document, 'exitFullscreen', { configurable: true, value: exitFullscreen });
    Object.defineProperty(document.documentElement, 'requestFullscreen', {
      configurable: true,
      value: requestFullscreen,
    });
  });

  afterEach(() => {
    document.body.classList.remove('overflow-hidden', 'mc-terminal-fullscreen');
  });

  it('enters browser fullscreen and leaves it again from the same button', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Toolbar connected onSend={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Enter fullscreen' }));
    expect(requestFullscreen).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole('button', { name: 'Exit fullscreen' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Exit fullscreen' }));
    expect(exitFullscreen).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole('button', { name: 'Enter fullscreen' })).toBeInTheDocument();
  });

  it('follows the browser when it leaves fullscreen on its own (Esc)', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Toolbar connected onSend={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Enter fullscreen' }));
    await screen.findByRole('button', { name: 'Exit fullscreen' });

    act(() => {
      fullscreenElement = null;
      document.dispatchEvent(new Event('fullscreenchange'));
    });
    expect(await screen.findByRole('button', { name: 'Enter fullscreen' })).toBeInTheDocument();
    expect(exitFullscreen).not.toHaveBeenCalled();
  });

  it('sends Ctrl+Alt+Del and Ctrl+Alt+F2 as their scancode sequences', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    renderWithProviders(<Toolbar connected onSend={onSend} />);

    await user.click(screen.getByRole('button', { name: 'Send keys' }));
    await user.click(await screen.findByRole('button', { name: 'Ctrl+Alt+Del' }));
    await user.click(screen.getByRole('button', { name: 'Ctrl+Alt+F2' }));

    expect(onSend).toHaveBeenCalledTimes(2);
    expect(spiceEvents(onSend.mock.calls[0]?.[0])).toEqual([
      { type: 'down', code: 0x1d },
      { type: 'down', code: 0x38 },
      { type: 'down', code: 0x53e0 },
      { type: 'up', code: 0xd3e0 },
      { type: 'up', code: 0xb8 },
      { type: 'up', code: 0x9d },
    ]);
    expect(spiceEvents(onSend.mock.calls[1]?.[0]).map((event) => event.code)).toEqual([
      0x1d, 0x38, 0x3c, 0xbc, 0xb8, 0x9d,
    ]);
  });

  it('types text into the guest as key presses', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    renderWithProviders(<Toolbar connected onSend={onSend} />);

    await user.click(screen.getByRole('button', { name: 'Send keys' }));
    await user.type(await screen.findByLabelText('Type text'), 'a');
    await user.click(screen.getByRole('button', { name: 'Type into the guest' }));

    await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
    expect(onSend.mock.calls[0]?.[0]).toEqual([
      { key: 'KeyA', down: true },
      { key: 'KeyA', down: false },
    ]);
  });

  it('offers the firmware keys the browser keeps for itself', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    renderWithProviders(<Toolbar connected onSend={onSend} />);

    await user.click(screen.getByRole('button', { name: 'Send keys' }));
    await user.click(await screen.findByRole('button', { name: 'Esc' }));
    expect(onSend).toHaveBeenCalledWith([
      { key: 'Escape', down: true },
      { key: 'Escape', down: false },
    ]);
    expect(screen.getByRole('button', { name: 'F2' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'F12' })).toBeInTheDocument();
  });

  it('keeps the send-keys menu disabled until the console is connected', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    renderWithProviders(<Toolbar connected={false} onSend={onSend} />);

    const trigger = screen.getByRole('button', { name: 'Send keys' });
    expect(trigger).toBeDisabled();
    await user.click(trigger);
    expect(screen.queryByRole('button', { name: 'Ctrl+Alt+Del' })).not.toBeInTheDocument();
    expect(onSend).not.toHaveBeenCalled();
  });
});
