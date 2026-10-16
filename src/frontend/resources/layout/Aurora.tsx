// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The aurora field — three slow blurred washes, fixed behind the app.
//
// This is not decoration for its own sake: it is what the glass panels
// blur. A translucent surface with a flat colour behind it reads as a
// slightly transparent card, not as glass. The drift is what makes the
// blur visibly *doing* something.
//
// It is `position: fixed`, `pointer-events: none`, and sits at z-index 0
// with the shell above it, so it never intercepts a click and never
// participates in layout. The colours come from `--aurora-a/b/c`, which
// every theme defines, so Gruvbox gets a Gruvbox aurora.
//
// The drift is one of the sanctioned motions and stops with the rest of
// them under `prefers-reduced-motion` or the in-app motion preference —
// see the gates at the end of app.css. The washes stay; only the movement
// stops, because a static gradient is still what the glass needs.

export function Aurora() {
  return (
    <div className="mc-aurora" aria-hidden="true">
      <span className="a" />
      <span className="b" />
      <span className="c" />
    </div>
  );
}
