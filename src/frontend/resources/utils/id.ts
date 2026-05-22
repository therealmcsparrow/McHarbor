// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

function byteToHex(byte: number): string {
  return byte.toString(16).padStart(2, '0');
}

export function createClientId(): string {
  const cryptoApi = globalThis.crypto;

  if (typeof cryptoApi?.randomUUID === 'function') {
    return cryptoApi.randomUUID();
  }

  if (typeof cryptoApi?.getRandomValues === 'function') {
    const bytes = new Uint8Array(16);
    cryptoApi.getRandomValues(bytes);

    // RFC 4122 version 4 UUID bits.
    const versionByte = bytes[6] ?? 0;
    const variantByte = bytes[8] ?? 0;
    bytes[6] = (versionByte & 0x0f) | 0x40;
    bytes[8] = (variantByte & 0x3f) | 0x80;

    const hex = Array.from(bytes, byteToHex);
    return [
      hex.slice(0, 4).join(''),
      hex.slice(4, 6).join(''),
      hex.slice(6, 8).join(''),
      hex.slice(8, 10).join(''),
      hex.slice(10, 16).join(''),
    ].join('-');
  }

  return `id-${Date.now().toString(16)}-${Math.random().toString(16).slice(2, 10)}`;
}
