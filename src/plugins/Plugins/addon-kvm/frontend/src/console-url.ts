// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** The gateway socket for one console ticket, on the page's own origin. */
export function consoleSocketUrl(ticket: string): string {
  const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${scheme}://${window.location.host}/api/compute/console/${ticket}`;
}
