// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { KeyStroke, SpiceKey } from './spice-keys';

const NAMED_KEYSYM: Partial<Record<SpiceKey, number>> = {
  Escape: 0xff1b,
  Backspace: 0xff08,
  Tab: 0xff09,
  Enter: 0xff0d,
  ControlLeft: 0xffe3,
  ShiftLeft: 0xffe1,
  AltLeft: 0xffe9,
  MetaLeft: 0xffeb,
  Space: 0x0020,
  PrintScreen: 0xff61,
  Delete: 0xffff,
  Minus: 0x002d,
  Equal: 0x003d,
  BracketLeft: 0x005b,
  BracketRight: 0x005d,
  Semicolon: 0x003b,
  Quote: 0x0027,
  Backquote: 0x0060,
  Backslash: 0x005c,
  Comma: 0x002c,
  Period: 0x002e,
  Slash: 0x002f,
};

/**
 * The X keysym of the unshifted key. QEMU takes the scancode from the
 * extended key event noVNC derives from `code`, so Shift+1 still arrives as
 * "!" in the guest; the keysym is only the fallback for a server without it.
 */
export function vncKeysym(key: SpiceKey): number {
  const named = NAMED_KEYSYM[key];
  if (named !== undefined) return named;
  const letter = /^Key([A-Z])$/.exec(key);
  if (letter) return letter[1]!.toLowerCase().charCodeAt(0);
  const digit = /^Digit(\d)$/.exec(key);
  if (digit) return digit[1]!.charCodeAt(0);
  const fn = /^F(\d{1,2})$/.exec(key);
  if (fn) return 0xffbe + Number(fn[1]) - 1;
  return 0;
}

/** The part of noVNC's RFB this needs, so a test can stand in for it. */
export interface VncKeyTarget {
  sendKey(keysym: number, code: string | null, down?: boolean): void;
}

/** Strokes into a noVNC session, in order; a missing session sends nothing. */
export function sendVncStrokes(rfb: VncKeyTarget | null, strokes: readonly KeyStroke[]): void {
  if (!rfb) return;
  for (const stroke of strokes) {
    rfb.sendKey(vncKeysym(stroke.key), stroke.key, stroke.down);
  }
}
