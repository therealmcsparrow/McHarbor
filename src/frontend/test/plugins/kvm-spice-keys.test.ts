// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it, vi } from 'vitest';
import {
  SPICE_VT_COMBOS,
  comboCaption,
  comboSequence,
  keyDownCode,
  keyUpCode,
  textSequence,
  textStrokes,
} from '../../../plugins/Plugins/addon-kvm/frontend/src/spice-keys';
import { sendVncStrokes, vncKeysym } from '../../../plugins/Plugins/addon-kvm/frontend/src/vnc-keys';

describe('addon-kvm SPICE scancodes', () => {
  it('sends Ctrl+Alt+Del as press-all then release-in-reverse, Delete as E0 53', () => {
    expect(comboSequence(['ControlLeft', 'AltLeft', 'Delete'])).toEqual([
      { type: 'down', code: 0x1d },
      { type: 'down', code: 0x38 },
      { type: 'down', code: 0x53e0 },
      { type: 'up', code: 0xd3e0 },
      { type: 'up', code: 0xb8 },
      { type: 'up', code: 0x9d },
    ]);
  });

  it('encodes extended keys low byte first, the way spice-server reads them', () => {
    expect(keyDownCode('MetaLeft')).toBe(0x5be0);
    expect(keyUpCode('MetaLeft')).toBe(0xdbe0);
    expect(keyDownCode('PrintScreen')).toBe(0x37e0);
    expect(keyUpCode('PrintScreen')).toBe(0xb7e0);
    expect(keyDownCode('F11')).toBe(0x57);
    expect(keyUpCode('Backspace')).toBe(0x8e);
  });

  it('offers Ctrl+Alt+F1 to F12', () => {
    expect(SPICE_VT_COMBOS.map((combo) => comboCaption(combo.keys))).toEqual(
      Array.from({ length: 12 }, (_, index) => `Ctrl+Alt+F${index + 1}`),
    );
    expect(comboSequence(SPICE_VT_COMBOS[0]!.keys).map((event) => event.code)).toEqual([
      0x1d, 0x38, 0x3b, 0xbb, 0xb8, 0x9d,
    ]);
  });

  it('types text on a US layout, holding Shift for shifted characters and skipping the rest', () => {
    const { events, skipped } = textSequence('aB!é');
    expect(skipped).toBe(1);
    expect(events).toEqual([
      { type: 'down', code: 0x1e },
      { type: 'up', code: 0x9e },
      { type: 'down', code: 0x2a },
      { type: 'down', code: 0x30 },
      { type: 'up', code: 0xb0 },
      { type: 'up', code: 0xaa },
      { type: 'down', code: 0x2a },
      { type: 'down', code: 0x02 },
      { type: 'up', code: 0x82 },
      { type: 'up', code: 0xaa },
    ]);
  });
});

describe('addon-kvm VNC keys', () => {
  it('sends the same chord over VNC as keysym plus code, pressed then released in reverse', () => {
    const sendKey = vi.fn();
    sendVncStrokes({ sendKey }, [
      { key: 'ControlLeft', down: true },
      { key: 'AltLeft', down: true },
      { key: 'Delete', down: true },
      { key: 'Delete', down: false },
      { key: 'AltLeft', down: false },
      { key: 'ControlLeft', down: false },
    ]);
    expect(sendKey.mock.calls).toEqual([
      [0xffe3, 'ControlLeft', true],
      [0xffe9, 'AltLeft', true],
      [0xffff, 'Delete', true],
      [0xffff, 'Delete', false],
      [0xffe9, 'AltLeft', false],
      [0xffe3, 'ControlLeft', false],
    ]);
  });

  it('maps every key the menu and the text box can produce to a keysym', () => {
    expect(vncKeysym('KeyA')).toBe(0x61);
    expect(vncKeysym('Digit1')).toBe(0x31);
    expect(vncKeysym('F1')).toBe(0xffbe);
    expect(vncKeysym('F12')).toBe(0xffc9);
    expect(vncKeysym('Escape')).toBe(0xff1b);
    expect(vncKeysym('MetaLeft')).toBe(0xffeb);
    const typed = textStrokes(`Hello, World! [1+2=3] {a|b} ~"';:<>?/_-\\`);
    expect(typed.skipped).toBe(0);
    expect(typed.strokes.filter((stroke) => vncKeysym(stroke.key) === 0)).toEqual([]);
  });

  it('sends nothing without a live session', () => {
    expect(() => sendVncStrokes(null, [{ key: 'KeyA', down: true }])).not.toThrow();
  });
});
