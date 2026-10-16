// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Top 10 pages that axe-core (or any other a11y scanner) should
// exercise on every CI run. Ordered by relative importance — bugs
// in these pages affect the most users.
//
// Adjust as new primary surfaces are added.
export const A11Y_PAGES: readonly string[] = [
  '/containers',
  '/images',
  '/volumes',
  '/networks',
  '/stacks',
  '/dashboard',
  '/settings',
  '/activity',
  '/audit',
  '/environments',
  '/settings?tab=themes',
  '/settings?tab=notifications',
  '/settings?tab=retention',
  '/profile?tab=appearance',
] as const;
