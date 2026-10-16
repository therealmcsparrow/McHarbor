// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { KEYBOARD_CAPTURE_ATTR, isEditableTarget, isInCaptureZone } from './keyboardCapture';

describe('keyboard capture', () => {
  it('treats anything inside a capture zone as captured, including a canvas', () => {
    const zone = document.createElement('div');
    zone.setAttribute(KEYBOARD_CAPTURE_ATTR, '');
    const canvas = document.createElement('canvas');
    zone.appendChild(canvas);
    document.body.appendChild(zone);
    expect(isInCaptureZone(canvas)).toBe(true);
    expect(isInCaptureZone(zone)).toBe(true);
    zone.remove();
  });

  it('leaves the rest of the page alone', () => {
    const div = document.createElement('div');
    document.body.appendChild(div);
    expect(isInCaptureZone(div)).toBe(false);
    expect(isInCaptureZone(null)).toBe(false);
    expect(isEditableTarget(div)).toBe(false);
    div.remove();
  });

  it('recognises fields the user types into', () => {
    expect(isEditableTarget(document.createElement('input'))).toBe(true);
    expect(isEditableTarget(document.createElement('textarea'))).toBe(true);
    expect(isEditableTarget(document.createElement('select'))).toBe(true);
  });
});
