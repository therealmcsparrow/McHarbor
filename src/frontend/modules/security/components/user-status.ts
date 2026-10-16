// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// One place that decides what state an account is in.
//
// Three states, and they are not the same thing:
//
//   locked   — too many failed sign-ins. Self-clearing at `lockedUntil`,
//              or immediately via "clear lockout". The user did nothing
//              wrong on purpose and will get back in.
//   disabled — an admin turned the account off. It stays off until an
//              admin turns it back on.
//   active   — neither.
//
// Both locked and disabled draw a lock; only disabled recolours the name,
// because disabled is the one that does not resolve on its own.

import type { UserItem } from '@resources/hooks/useUsers';

export type UserAccountState = 'active' | 'locked' | 'disabled';

/** True while `lockedUntil` is set and still in the future. */
export function isUserLockedOut(user: Pick<UserItem, 'lockedUntil'>): boolean {
  if (!user.lockedUntil) return false;
  const until = Date.parse(user.lockedUntil);
  return Number.isFinite(until) && until > Date.now();
}

/**
 * Disabled outranks locked: an admin decision is the reason the user
 * cannot get in, and clearing the lockout would not change that.
 */
export function userAccountState(user: Pick<UserItem, 'isActive' | 'lockedUntil'>): UserAccountState {
  if (!user.isActive) return 'disabled';
  return isUserLockedOut(user) ? 'locked' : 'active';
}
