// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Types for spice-html5, which ships none of its own.
 *
 * Only the surface `spice-session.ts` drives is declared, for the same reason
 * as `novnc.d.ts`: an upstream rename then fails the build instead of drifting.
 * `channel_uri` is not upstream's: it is the hook the build's
 * `spiceChannelUrls()` transform adds, so each channel gets its own ticket.
 */
declare module '@spice-project/spice-html5/src/main.js' {
  export interface SpiceMainConnOptions {
    uri: string;
    /** The gateway authenticates with the ticket; QEMU runs with ticketing off. */
    password: string;
    /** Id of an element in the document where the display canvas is created. */
    screen_id: string;
    onerror?: (error: Error) => void;
    onsuccess?: () => void;
    onagent?: (connection: SpiceMainConn) => void;
  }

  export class SpiceMainConn {
    constructor(options: SpiceMainConnOptions);
    /** Returns the URL the next channel socket opens on. */
    channel_uri?: () => string;
    /** The inputs channel, once the server has offered it. */
    inputs?: SpiceInputsConn;
    /** True once the guest's spice-vdagent is attached; agent messages are dropped until then. */
    agent_connected?: boolean;
    /** Sends VD_AGENT_MONITORS_CONFIG for one monitor: the guest's vdagent applies it. */
    resize_window(flags: number, width: number, height: number, depth: number, x: number, y: number): void;
    stop(): void;
  }

  /** Only `send_msg` and the readiness flag are driven, for sending keys. */
  export interface SpiceInputsConn {
    state: string;
    send_msg(msg: import('@spice-project/spice-html5/src/spicemsg.js').SpiceMiniData): void;
  }
}

declare module '@spice-project/spice-html5/src/spicemsg.js' {
  export interface SpiceMsgBody {
    to_buffer(buffer: ArrayBuffer, at?: number): number;
    buffer_size(): number;
  }

  export class SpiceMiniData {
    build_msg(type: number, body: SpiceMsgBody): void;
  }

  /** Down and up share one wire layout: a u32 scancode, so one class serves both. */
  export class SpiceMsgcKeyDown implements SpiceMsgBody {
    code: number;
    to_buffer(buffer: ArrayBuffer, at?: number): number;
    buffer_size(): number;
  }
}

declare module '@spice-project/spice-html5/src/enums.js' {
  export const Constants: {
    SPICE_MSGC_INPUTS_KEY_DOWN: number;
    SPICE_MSGC_INPUTS_KEY_UP: number;
  };
}
