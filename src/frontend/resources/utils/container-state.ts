// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// How a container state should look, in one place.
//
// This already existed and was already the right idea; what it lacked was a
// `tone`. Callers had a `badgeVariant` and a `dotClassName` and nothing that
// named the *meaning*, so anything that needed a text colour, a border or a
// ring hand-rolled one — which is how the same "running" container ended up
// teal on a card, green in a row and emerald in a widget.
//
// Adding the tone lets everything route through `status-palette.ts`, and
// `StatusIndicator` then renders it identically everywhere.

import type { StatusTone } from '@resources/theme/status-palette';
import { STATUS_COLORS } from '@resources/theme/status-palette';

export type ContainerStateBadgeVariant = 'success' | 'destructive' | 'warning' | 'secondary';

export interface ContainerStatePresentation {
  /** Semantic tone. The single source for colour. */
  tone: StatusTone;
  badgeVariant: ContainerStateBadgeVariant;
  dotClassName: string;
}

function presentation(tone: StatusTone, badgeVariant: ContainerStateBadgeVariant): ContainerStatePresentation {
  return { tone, badgeVariant, dotClassName: STATUS_COLORS[tone].dot };
}

const runningPresentation = presentation('success', 'success');
const warningPresentation = presentation('warning', 'warning');
const dangerPresentation = presentation('danger', 'destructive');
const neutralPresentation = presentation('neutral', 'secondary');

const statePresentations: Record<string, ContainerStatePresentation> = {
  running: runningPresentation,
  paused: warningPresentation,
  restarting: warningPresentation,
  created: neutralPresentation,
  exited: dangerPresentation,
  removing: dangerPresentation,
  dead: presentation('critical', 'destructive'),
};

const healthPresentations: Record<string, ContainerStatePresentation> = {
  healthy: runningPresentation,
  unhealthy: dangerPresentation,
  starting: warningPresentation,
};

const fallbackPresentation = neutralPresentation;

export function containerStatePresentation(
  state: string | null | undefined,
  health?: string | null,
): ContainerStatePresentation {
  if (state === 'running' && health && health !== 'healthy') {
    return healthPresentations[health] ?? runningPresentation;
  }
  return statePresentations[state ?? ''] ?? fallbackPresentation;
}

export function containerHealthPresentation(health: string | null | undefined): ContainerStatePresentation {
  return healthPresentations[health ?? ''] ?? fallbackPresentation;
}
