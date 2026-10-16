// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export function guestNameErrorKey(nameValue: string): string | null {
  if (nameValue === '') return 'err.vm.name_required';
  if (Array.from(nameValue).length > 120) return 'err.vm.name_too_long';
  for (const char of nameValue) {
    const code = char.codePointAt(0) ?? 0;
    if (code < 0x20 || code === 0x7f) return 'err.vm.name_invalid';
  }
  return null;
}
