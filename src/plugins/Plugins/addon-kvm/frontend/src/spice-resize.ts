// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RESIZE_DEBOUNCE_MS, debounce, fitScale, spiceMonitorSize, type ConsoleDisplayMode } from './console-resize';

export interface SpiceResizeTarget {
  /** Asks the guest for a monitor size; false when no spice-vdagent has connected. */
  resize: (width: number, height: number) => boolean;
}

/**
 * Keeps a SPICE display the size of its box.
 *
 * spice-html5's own resize helper measures the browser window rather than an
 * element, which is wrong for a console inside a page, so this measures the
 * box and sends the same monitor config through `resize_window`. The canvas
 * is fitted with a transform rather than CSS width: spice-html5 reads pointer
 * positions from `offsetX`, which browsers report in the transformed element's
 * own coordinates, so the pointer still lands where it is drawn.
 */
export function watchSpiceResize(options: {
  viewport: HTMLElement;
  screen: HTMLElement;
  mode: () => ConsoleDisplayMode;
  target: () => SpiceResizeTarget | null;
}): { refresh: () => void; applyNow: () => void; dispose: () => void } {
  const { viewport: boxEl, screen: screenEl, mode, target } = options;

  const applyNow = () => {
    const width = boxEl.clientWidth;
    const height = boxEl.clientHeight;
    const canvas = screenEl.querySelector('canvas');
    const scale = canvas ? fitScale(width, height, canvas.width, canvas.height) : 1;
    screenEl.style.transformOrigin = '0 0';
    screenEl.style.transform = scale === 1 ? '' : `scale(${scale})`;

    if (mode() !== 'resize') return;
    const size = spiceMonitorSize(width, height);
    if (size) target()?.resize(size.width, size.height);
  };

  const debounced = debounce(applyNow, RESIZE_DEBOUNCE_MS);
  let observer: ResizeObserver | null = null;
  if (typeof ResizeObserver !== 'undefined') {
    // The screen is observed too: spice-html5 sets its height to the guest's
    // surface whenever the guest changes resolution, which is when the fit changes.
    observer = new ResizeObserver(() => debounced.call());
    observer.observe(boxEl);
    observer.observe(screenEl);
  }

  return {
    refresh: debounced.call,
    applyNow,
    dispose: () => {
      debounced.cancel();
      observer?.disconnect();
      screenEl.style.transform = '';
    },
  };
}
