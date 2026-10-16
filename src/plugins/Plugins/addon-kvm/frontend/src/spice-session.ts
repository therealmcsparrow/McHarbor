// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The browser SPICE client, loaded only when an operator picks SPICE.
 *
 * spice-html5 is ~9k lines that most sessions never need, so it arrives through
 * a dynamic import rather than with the bundle's top-level code.
 */

import type { SpiceKeyEvent } from './spice-keys';

type SpiceModules = {
  main: typeof import('@spice-project/spice-html5/src/main.js');
  msg: typeof import('@spice-project/spice-html5/src/spicemsg.js');
  enums: typeof import('@spice-project/spice-html5/src/enums.js');
};

let loading: Promise<SpiceModules> | null = null;

function loadSpice(): Promise<SpiceModules> {
  loading ??= Promise.all([
    import('@spice-project/spice-html5/src/main.js'),
    import('@spice-project/spice-html5/src/spicemsg.js'),
    import('@spice-project/spice-html5/src/enums.js'),
  ]).then(([main, msg, enums]) => ({ main, msg, enums }));
  return loading;
}

/**
 * How many channel tickets to ask the gateway for.
 *
 * A QEMU guest offers main, display, inputs, cursor, playback and record, plus
 * a port per agent channel. Unused tickets simply expire, so asking for the
 * gateway's maximum costs nothing and never leaves a channel without a URL.
 */
export const SPICE_CHANNELS = 8;

export interface SpiceHandle {
  stop: () => void;
  /** Sends the events in order; false when the inputs channel is not ready yet. */
  sendKeys: (events: readonly SpiceKeyEvent[]) => boolean;
  /** Asks the guest for one monitor of this size; false until spice-vdagent is attached. */
  resize: (width: number, height: number) => boolean;
}

let screenSeq = 0;

/**
 * Open a SPICE session into `screen` over one URL per channel.
 *
 * `urls[0]` carries the main channel; each later channel takes the next URL
 * through `channel_uri`, the hook the build adds to spice-html5. The screen
 * element needs an id because spice-html5 finds it with `getElementById`.
 */
export async function openSpice(options: {
  screenEl: HTMLElement;
  urls: string[];
  onConnected: () => void;
  onError: (message: string) => void;
  /** Called when the guest's spice-vdagent attaches, which is when resizing starts to work. */
  onAgent?: () => void;
}): Promise<SpiceHandle> {
  const { main, msg, enums } = await loadSpice();
  const { SpiceMainConn } = main;
  const { screenEl, urls, onConnected, onError, onAgent } = options;
  if (!screenEl.id) {
    screenSeq += 1;
    screenEl.id = `mcharbor-spice-screen-${screenSeq}`;
  }

  let next = 1;
  const connection = new SpiceMainConn({
    uri: urls[0] ?? '',
    password: '',
    screen_id: screenEl.id,
    onsuccess: onConnected,
    onerror: (error) => onError(error.message),
    onagent: () => onAgent?.(),
  });
  // Past the last ticket there is no URL a gateway would accept; handing back
  // the main one makes that channel fail visibly rather than hang.
  connection.channel_uri = () => {
    const url = urls[next] ?? urls[0] ?? '';
    next += 1;
    return url;
  };

  // Built from spice-html5's own message classes, the way its keyboard handler
  // does, rather than its sendCtrlAltDel, which reads a variable it never defines.
  const sendKeys = (events: readonly SpiceKeyEvent[]) => {
    const inputs = connection.inputs;
    if (!inputs || inputs.state !== 'ready') return false;
    for (const keyEvent of events) {
      const body = new msg.SpiceMsgcKeyDown();
      body.code = keyEvent.code;
      const message = new msg.SpiceMiniData();
      message.build_msg(
        keyEvent.type === 'down'
          ? enums.Constants.SPICE_MSGC_INPUTS_KEY_DOWN
          : enums.Constants.SPICE_MSGC_INPUTS_KEY_UP,
        body,
      );
      inputs.send_msg(message);
    }
    return true;
  };

  // resize_window rather than spice-html5's resize helper, which sizes to the
  // browser window instead of the console box.
  const resize = (width: number, height: number) => {
    if (!connection.agent_connected) return false;
    connection.resize_window(0, width, height, 32, 0, 0);
    return true;
  };

  return {
    sendKeys,
    resize,
    stop: () => {
      try {
        connection.stop();
      } catch {
        // spice-html5 throws when a channel never finished connecting; the sockets are gone either way.
      }
      screenEl.replaceChildren();
    },
  };
}
