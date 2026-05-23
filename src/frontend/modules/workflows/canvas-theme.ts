// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Centralized canvas color tokens.
 *
 * Category A tokens use `foreground` so they adapt to light/dark theme.
 * To make the canvas always-dark, change `foreground` → `white` in the
 * opacity variants below.
 *
 * Category B/C tokens are fixed (white on colored backgrounds, toggle knobs)
 * and should NOT be changed.
 */
export const ct = {
  // --- Category A: Theme-aware opacity variants ---
  text10: "text-foreground/10",
  text20: "text-foreground/20",
  text25: "text-foreground/25",
  text30: "text-foreground/30",
  text40: "text-foreground/40",
  text50: "text-foreground/50",
  text60: "text-foreground/60",
  text80: "text-foreground/80",
  border30: "border-foreground/30",
  border60: "border-foreground/60",
  bg20: "bg-foreground/20",

  // --- Category A: Theme-aware surface colors ---
  // Canvas background, node bodies, input fields on canvas
  canvasBg: "bg-background",
  nodeBg: "bg-muted",
  portBg: "bg-muted/50",
  gridDot: "rgba(128,128,128,0.15)",

  // --- Category B: Fixed contrast (on colored backgrounds) ---
  nodeHeaderText: "text-primary-foreground",
  activeBtnText: "text-primary-foreground",
  runBtnText: "text-primary-foreground",

  // --- Category C: Fixed structural ---
  toggleKnob: "bg-background",
  backdrop: "bg-background/40",
  tooltipBg: "bg-popover/95",
} as const;
