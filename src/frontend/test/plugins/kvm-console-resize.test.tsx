// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The plugin's ambient types for noVNC and spice-html5 sit outside this project's include.
// eslint-disable-next-line @typescript-eslint/triple-slash-reference -- ambient module declarations, nothing to import
/// <reference path="../../../plugins/Plugins/addon-kvm/frontend/src/novnc.d.ts" />
// eslint-disable-next-line @typescript-eslint/triple-slash-reference -- ambient module declarations, nothing to import
/// <reference path="../../../plugins/Plugins/addon-kvm/frontend/src/spice.d.ts" />

import { screen } from '@testing-library/react';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  setConsoleDisplayMode,
  startConsole,
  stopConsole,
} from '../../../plugins/Plugins/addon-kvm/frontend/src/console-sessions';
import { watchSpiceResize } from '../../../plugins/Plugins/addon-kvm/frontend/src/spice-resize';
import { ConsoleResizeHint } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/ConsoleResizeHint';
import { SpiceConsolePane } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/SpiceConsolePane';
import type { VM } from '../../../plugins/Plugins/addon-kvm/frontend/src/types';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';

const rfbInstances = vi.hoisted(() => [] as Array<{ resizeSession: boolean; scaleViewport: boolean }>);

// The path, not the bare specifier: noVNC resolves from the plugin's own node_modules.
vi.mock('../../../plugins/Plugins/addon-kvm/frontend/node_modules/@novnc/novnc/lib/rfb.js', () => {
  class FakeRFB extends EventTarget {
    resizeSession = false;
    scaleViewport = false;
    constructor() {
      super();
      rfbInstances.push(this);
    }
    disconnect() {
      this.dispatchEvent(new Event('disconnect'));
    }
  }
  return { default: FakeRFB };
});

type ObserverCallback = () => void;
const observers: ObserverCallback[] = [];

class FakeResizeObserver {
  constructor(callback: ObserverCallback) {
    observers.push(callback);
  }
  observe() {}
  disconnect() {}
}

function sized(width: number, height: number): HTMLDivElement {
  const el = document.createElement('div');
  Object.defineProperty(el, 'clientWidth', { configurable: true, value: width });
  Object.defineProperty(el, 'clientHeight', { configurable: true, value: height });
  return el;
}

describe('addon-kvm console resize', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  beforeEach(() => {
    rfbInstances.length = 0;
    observers.length = 0;
    vi.stubGlobal('ResizeObserver', FakeResizeObserver);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('asks noVNC to resize the guest in resize mode, and only to scale in scale mode', () => {
    startConsole('vm-resize', 'wss://example.invalid/a', 1);
    expect(rfbInstances[0]).toMatchObject({ resizeSession: true, scaleViewport: true });
    stopConsole('vm-resize');

    setConsoleDisplayMode('vm-scale', 'scale');
    startConsole('vm-scale', 'wss://example.invalid/b', 1);
    expect(rfbInstances[1]).toMatchObject({ resizeSession: false, scaleViewport: true });
    stopConsole('vm-scale');
  });

  it('sends one SPICE monitor size per burst of container resizes, rounded to 8', () => {
    vi.useFakeTimers();
    const resize = vi.fn(() => true);
    const watcher = watchSpiceResize({
      viewport: sized(1283, 805),
      screen: document.createElement('div'),
      mode: () => 'resize',
      target: () => ({ resize }),
    });

    for (const callback of observers) callback();
    for (const callback of observers) callback();
    vi.advanceTimersByTime(150);
    observers[0]?.();
    vi.advanceTimersByTime(199);
    expect(resize).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(resize).toHaveBeenCalledTimes(1);
    expect(resize).toHaveBeenCalledWith(1280, 800);
    watcher.dispose();
  });

  it('never asks a SPICE guest to resize in scale mode', () => {
    vi.useFakeTimers();
    const resize = vi.fn(() => true);
    const watcher = watchSpiceResize({
      viewport: sized(1024, 768),
      screen: document.createElement('div'),
      mode: () => 'scale',
      target: () => ({ resize }),
    });
    for (const callback of observers) callback();
    vi.advanceTimersByTime(1000);
    expect(resize).not.toHaveBeenCalled();
    watcher.dispose();
  });

  it('says why the guest cannot follow the window, and names the agent it needs', () => {
    const { rerender } = renderWithProviders(
      <ConsoleResizeHint kind="vnc" mode="resize" adapter="std" hostResize guestUnanswered={false} />,
    );
    expect(screen.getByRole('status')).toHaveTextContent('display adapter (std)');

    rerender(
      <ConsoleResizeHint kind="vnc" mode="resize" adapter="virtio-gpu" hostResize={false} guestUnanswered={false} />,
    );
    expect(screen.getByRole('status')).toHaveTextContent('QEMU 6.0 or later');

    rerender(<ConsoleResizeHint kind="spice" mode="resize" adapter="qxl" hostResize guestUnanswered />);
    expect(screen.getByRole('status')).toHaveTextContent('spice-vdagent');
    expect(screen.getByRole('status')).toHaveTextContent('qemu-guest-agent is a different agent');

    rerender(<ConsoleResizeHint kind="vnc" mode="resize" adapter="virtio-gpu" hostResize guestUnanswered={false} />);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    rerender(<ConsoleResizeHint kind="vnc" mode="scale" adapter="std" hostResize guestUnanswered />);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('keeps keystrokes on the display inside the capture zone, and the toolbar outside it', () => {
    grantPermissions(['vm.view', 'vm.console']);
    const vm = {
      id: 'vm-1',
      environmentId: 'env-1',
      name: 'guest',
      state: 'running',
      displayAdapter: 'virtio-gpu',
    } as VM;
    renderWithProviders(
      <SpiceConsolePane
        vm={vm}
        slots={{ leading: null, trailing: null }}
        display={{ mode: 'resize', fullscreen: false, hostResize: true }}
      />,
    );

    const display = screen.getByRole('application', { name: 'SPICE' });
    expect(display).toHaveAttribute('tabindex', '0');
    expect(display.closest('[data-keyboard-capture]')).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Send keys' }).closest('[data-keyboard-capture]')).toBeNull();
  });
});
