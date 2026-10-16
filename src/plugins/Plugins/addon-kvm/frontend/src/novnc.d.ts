// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Types for noVNC, which ships none of its own.
 *
 * The version is pinned to 1.6.0 rather than caret-ranged: 1.7 introduced a
 * top-level await in its WebCodecs probe, and top-level await cannot be
 * expressed in a UMD bundle — which is the format the host's plugin loader
 * requires. 1.6 ships transpiled modules under lib/ and has no `exports` field.
 *
 * Only the surface this plugin actually drives is declared. Writing the whole
 * RFB API here would be a second, unverified copy of upstream's behaviour that
 * would drift silently; declaring the five members the console uses means an
 * upstream rename fails the build instead.
 */
declare module '@novnc/novnc/lib/rfb.js' {
  export interface RFBOptions {
    /** Credentials for a server that asks for them. Unused here: the gateway authenticates. */
    credentials?: { username?: string; password?: string; target?: string };
    /** Sub-path on the WebSocket URL. Unused: the ticket is the whole path. */
    repeaterID?: string;
    shared?: boolean;
    wsProtocols?: string[];
  }

  export default class RFB extends EventTarget {
    constructor(target: HTMLElement, url: string, options?: RFBOptions);

    /** Scale the framebuffer to the container rather than clipping it. */
    scaleViewport: boolean;
    /** Ask the guest to match its resolution to the container. */
    resizeSession: boolean;
    viewOnly: boolean;

    disconnect(): void;
    sendCtrlAltDel(): void;
    sendKey(keysym: number, code: string | null, down?: boolean): void;
    focus(): void;
    clipboardPasteFrom(text: string): void;
  }
}
