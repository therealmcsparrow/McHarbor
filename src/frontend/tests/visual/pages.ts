// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Top routes that the visual regression suite screenshots in both
// light and dark mode. Keep this list focused — every entry doubles
// the runtime (one capture per theme) and inflates the golden
// baseline repository.
//
// Choose pages that:
//   - cover primary surfaces (lists, detail, settings, dashboard)
//   - exercise unique layout primitives (tabs, table, code editor)
//   - are unlikely to churn on every release (no dashboard widgets)
//
// Adjust as new primary surfaces are added.
export const VISUAL_PAGES: readonly string[] = [
  '/containers',
  '/images',
  '/volumes',
  '/networks',
  '/stacks',
  '/dashboard',
  '/settings',
  '/activity',
  '/environments',
] as const;

export const VISUAL_THEMES: readonly ('dark' | 'light')[] = ['dark', 'light'] as const;
