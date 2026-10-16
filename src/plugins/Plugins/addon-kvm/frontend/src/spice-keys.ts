// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * PC set-1 scancodes, by `KeyboardEvent.code` name. An extended key carries
 * its E0 prefix in the high byte (`0xe053` is E0 53, Delete).
 *
 * Kept here rather than read from spice-html5: its map is keyed on the
 * deprecated `keyCode`, and its encoding of extended keys turns `0xe053` into
 * three bytes the guest does not recognise.
 */
const SCANCODE = {
  Escape: 0x01,
  Digit1: 0x02,
  Digit2: 0x03,
  Digit3: 0x04,
  Digit4: 0x05,
  Digit5: 0x06,
  Digit6: 0x07,
  Digit7: 0x08,
  Digit8: 0x09,
  Digit9: 0x0a,
  Digit0: 0x0b,
  Minus: 0x0c,
  Equal: 0x0d,
  Backspace: 0x0e,
  Tab: 0x0f,
  KeyQ: 0x10,
  KeyW: 0x11,
  KeyE: 0x12,
  KeyR: 0x13,
  KeyT: 0x14,
  KeyY: 0x15,
  KeyU: 0x16,
  KeyI: 0x17,
  KeyO: 0x18,
  KeyP: 0x19,
  BracketLeft: 0x1a,
  BracketRight: 0x1b,
  Enter: 0x1c,
  ControlLeft: 0x1d,
  KeyA: 0x1e,
  KeyS: 0x1f,
  KeyD: 0x20,
  KeyF: 0x21,
  KeyG: 0x22,
  KeyH: 0x23,
  KeyJ: 0x24,
  KeyK: 0x25,
  KeyL: 0x26,
  Semicolon: 0x27,
  Quote: 0x28,
  Backquote: 0x29,
  ShiftLeft: 0x2a,
  Backslash: 0x2b,
  KeyZ: 0x2c,
  KeyX: 0x2d,
  KeyC: 0x2e,
  KeyV: 0x2f,
  KeyB: 0x30,
  KeyN: 0x31,
  KeyM: 0x32,
  Comma: 0x33,
  Period: 0x34,
  Slash: 0x35,
  AltLeft: 0x38,
  Space: 0x39,
  F1: 0x3b,
  F2: 0x3c,
  F3: 0x3d,
  F4: 0x3e,
  F5: 0x3f,
  F6: 0x40,
  F7: 0x41,
  F8: 0x42,
  F9: 0x43,
  F10: 0x44,
  F11: 0x57,
  F12: 0x58,
  PrintScreen: 0xe037,
  Delete: 0xe053,
  MetaLeft: 0xe05b,
} as const;

export type SpiceKey = keyof typeof SCANCODE;

export type SpiceKeyEvent = { type: 'down' | 'up'; code: number };

const BREAK = 0x80;
const E0 = 0xe0;

function isExtended(scancode: number) {
  return (scancode & 0xff00) === 0xe000;
}

/**
 * The value of `SpiceMsgcKeyDown.code` for a press. spice-server reads it as up
 * to four bytes, low byte first, stopping at a zero byte, so E0 53 is `0x53e0`.
 */
export function keyDownCode(key: SpiceKey): number {
  const scancode = SCANCODE[key];
  return isExtended(scancode) ? E0 | ((scancode & 0xff) << 8) : scancode;
}

/** The release of `key`: the same bytes with the break bit set on the last one. */
export function keyUpCode(key: SpiceKey): number {
  const scancode = SCANCODE[key];
  return isExtended(scancode) ? E0 | (((scancode & 0xff) | BREAK) << 8) : scancode | BREAK;
}

/**
 * One key going down or up, by name. The keys menu speaks in these so SPICE
 * (scancodes) and VNC (keysyms) can each put them on their own wire.
 */
export type KeyStroke = { key: SpiceKey; down: boolean };

/**
 * A chord as the guest expects it: every key pressed in order, then released
 * in reverse, so modifiers are held for the whole of the final key.
 */
export function comboStrokes(keys: readonly SpiceKey[]): KeyStroke[] {
  return [
    ...keys.map((key): KeyStroke => ({ key, down: true })),
    ...[...keys].reverse().map((key): KeyStroke => ({ key, down: false })),
  ];
}

/** Strokes as SPICE inputs-channel events. */
export function spiceEvents(strokes: readonly KeyStroke[]): SpiceKeyEvent[] {
  return strokes.map((stroke) =>
    stroke.down ? { type: 'down', code: keyDownCode(stroke.key) } : { type: 'up', code: keyUpCode(stroke.key) },
  );
}

export function comboSequence(keys: readonly SpiceKey[]): SpiceKeyEvent[] {
  return spiceEvents(comboStrokes(keys));
}

export type SpiceKeyCombo = {
  id: string;
  keys: readonly SpiceKey[];
  /** Combos whose name is prose rather than key caps; resolved through `t()`. */
  named?: 'windows' | 'printScreen';
};

const FUNCTION_KEYS = ['F1', 'F2', 'F3', 'F4', 'F5', 'F6', 'F7', 'F8', 'F9', 'F10', 'F11', 'F12'] as const;

export const SPICE_KEY_COMBOS: readonly SpiceKeyCombo[] = [
  { id: 'ctrl-alt-del', keys: ['ControlLeft', 'AltLeft', 'Delete'] },
  { id: 'ctrl-alt-backspace', keys: ['ControlLeft', 'AltLeft', 'Backspace'] },
  { id: 'alt-tab', keys: ['AltLeft', 'Tab'] },
  { id: 'alt-f4', keys: ['AltLeft', 'F4'] },
  { id: 'windows', keys: ['MetaLeft'], named: 'windows' },
  { id: 'print-screen', keys: ['PrintScreen'], named: 'printScreen' },
  // What a firmware watches for: Esc enters OVMF, F2 and F12 the setup and boot
  // menus. The browser keeps Esc for leaving fullscreen, so it has to be sent.
  { id: 'escape', keys: ['Escape'] },
  { id: 'f2', keys: ['F2'] },
  { id: 'f12', keys: ['F12'] },
];

/** Ctrl+Alt+F1…F12: the virtual-terminal switch on a Linux guest. */
export const SPICE_VT_COMBOS: readonly SpiceKeyCombo[] = FUNCTION_KEYS.map((fn) => ({
  id: `ctrl-alt-${fn.toLowerCase()}`,
  keys: ['ControlLeft', 'AltLeft', fn],
}));

/** Key caps as printed on the keyboard; listed in donottranslate.md. */
const KEY_CAP: Partial<Record<SpiceKey, string>> = {
  Escape: 'Esc',
  ControlLeft: 'Ctrl',
  AltLeft: 'Alt',
  Delete: 'Del',
  Backspace: 'Backspace',
  Tab: 'Tab',
};

export function comboCaption(keys: readonly SpiceKey[]): string {
  return keys.map((key) => KEY_CAP[key] ?? key).join('+');
}

/** Characters of a US keyboard: the key, and whether Shift is held for it. */
function usLayout(): Map<string, { key: SpiceKey; shift: boolean }> {
  const layout = new Map<string, { key: SpiceKey; shift: boolean }>();
  for (const letter of 'abcdefghijklmnopqrstuvwxyz') {
    const key = `Key${letter.toUpperCase()}` as SpiceKey;
    layout.set(letter, { key, shift: false });
    layout.set(letter.toUpperCase(), { key, shift: true });
  }
  const rows: Array<[SpiceKey, string, string]> = [
    ['Digit1', '1', '!'],
    ['Digit2', '2', '@'],
    ['Digit3', '3', '#'],
    ['Digit4', '4', '$'],
    ['Digit5', '5', '%'],
    ['Digit6', '6', '^'],
    ['Digit7', '7', '&'],
    ['Digit8', '8', '*'],
    ['Digit9', '9', '('],
    ['Digit0', '0', ')'],
    ['Minus', '-', '_'],
    ['Equal', '=', '+'],
    ['BracketLeft', '[', '{'],
    ['BracketRight', ']', '}'],
    ['Semicolon', ';', ':'],
    ['Quote', "'", '"'],
    ['Backquote', '`', '~'],
    ['Backslash', '\\', '|'],
    ['Comma', ',', '<'],
    ['Period', '.', '>'],
    ['Slash', '/', '?'],
  ];
  for (const [key, plain, shifted] of rows) {
    layout.set(plain, { key, shift: false });
    layout.set(shifted, { key, shift: true });
  }
  layout.set(' ', { key: 'Space', shift: false });
  layout.set('\n', { key: 'Enter', shift: false });
  layout.set('\t', { key: 'Tab', shift: false });
  return layout;
}

let layoutCache: Map<string, { key: SpiceKey; shift: boolean }> | null = null;

/**
 * `text` as key presses on a US layout. A character with no key there is
 * skipped and counted, because a guest with another layout would turn a
 * guessed key into the wrong character anyway.
 */
export function textStrokes(text: string): { strokes: KeyStroke[]; skipped: number } {
  layoutCache ??= usLayout();
  const strokes: KeyStroke[] = [];
  let skipped = 0;
  for (const char of text.replace(/\r\n?/g, '\n')) {
    const entry = layoutCache.get(char);
    if (!entry) {
      skipped += 1;
      continue;
    }
    strokes.push(...comboStrokes(entry.shift ? ['ShiftLeft', entry.key] : [entry.key]));
  }
  return { strokes, skipped };
}

export function textSequence(text: string): { events: SpiceKeyEvent[]; skipped: number } {
  const { strokes, skipped } = textStrokes(text);
  return { events: spiceEvents(strokes), skipped };
}
