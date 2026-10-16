// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// widget-kit — the handful of shapes every dashboard tile is built from.
//
// Every widget used to carry its own header, its own empty state and its own
// list markup, and they drifted: one had a scrollbar, the next clipped, a
// third put three small boxes in the corner of a 400px tile and left the
// rest blank. These primitives fix the *fill* problem once:
//
//   * `WidgetFrame`   — header (title, badge, action) + a body that is a
//                       `min-h-0 flex-1` flex column, so children stretch.
//   * `useFittedRows` — measures the body and says how many fixed-height
//                       rows fit. Lists render exactly that many and say
//                       "+N more" for the rest. No scrollbars, no tails.
//   * `WidgetRow`     — one list row, 40px, optional click-through.
//   * `WidgetTile`    — a stat cell that grows to fill its grid track.
//   * `SegmentBar`    — the proportional bar fleet-health introduced.
//   * `WidgetEmpty`   — icon + sentence, centred in whatever is left.
//
// Deliberately free of module-level state and host singletons: plugin
// bundles import this file too (through the `@modules` alias) and compile
// their own copy, which is fine for pure presentation.
//
// This file is the public entry point; the primitives live in the sibling
// `widget-kit-*` files, grouped by shape.

export { TONE_SOFT, TONE_SOLID, TONE_TEXT, type Tone } from './widget-kit-tones';
export { WidgetFrame, WidgetBadge, WidgetEmpty, WidgetLoading, WidgetFooterLink } from './widget-kit-frame';
export { useFittedRows } from '../hooks/useFittedRows';
export { ROW_HEIGHT, WidgetRow, WidgetList, ToneDot, IconTile } from './widget-kit-rows';
export { WidgetTile, SegmentBar, relativeTime, type Segment } from './widget-kit-tiles';
