// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The states an install's licensing can be in. None of them gates
 * anything: an expired or suspended licence is a billing conversation,
 * not an outage.
 */
export type LicenseState =
  | 'ok'
  | 'over'
  | 'expired'
  | 'unlicensed'
  | 'suspended'
  | 'revoked'
  | 'pending_approval'
  | 'not_started';

/** The banner kinds the backend will ask for. */
export type LicenseNoticeKind =
  | 'overage'
  | 'expired'
  | 'unlicensed'
  | 'suspended'
  | 'revoked'
  | 'pending_approval'
  | 'not_started';

/**
 * What `GET /license/status` returns. Readable by every authenticated
 * user, because every user sees the banner this drives.
 */
export type LicenseStatus = {
  tier: string;
  state: LicenseState;
  serversUsed: number;
  serversLimit: number;
  expiresAt?: string;
  /** Surfaced well before it reaches zero — that is the whole point of it. */
  daysRemaining: number;
  noticeDue: boolean;
  noticeKind?: LicenseNoticeKind;
  noticePhase?: string;
  contactUrl?: string;
  offlineDays: number;
  sampledAt?: string;
};

/** The seat allowance. `unlimited` outranks `total`. */
export type LicenseSeats = {
  unlimited: boolean;
  total: number;
  used: number;
};

/** The validity window. `perpetual` outranks `expiresAt`. */
export type LicenseTerm = {
  perpetual: boolean;
  startsAt?: string;
  expiresAt?: string;
  daysRemaining: number;
  autoRenew: boolean;
};

/** The licence record itself, behind `license.view`. */
export type LicenseRecord = {
  id: string;
  /** The full key never leaves the backend. */
  keyMasked: string;
  serverLicenseId?: string;
  tier: string;
  status: string;
  /** The licence server's verdict — this is what decides, not any status code. */
  valid: boolean;
  reason?: string;
  seats: LicenseSeats;
  term: LicenseTerm;
  instanceRegistered: boolean;
  holdsSeat: boolean;
  activatedAt: string;
  lastCheckedAt?: string;
  lastError?: string;
};

/** What `GET /license` returns. */
export type LicenseDetail = {
  license: LicenseRecord | null;
  /**
   * This installation's seat. Not a credential — releasing the seat from
   * the mcharbor.eu dashboard addresses it by exactly this value, so it
   * is shown in full.
   */
  instanceId: string;
  instanceType: string;
  hostname?: string;
  serverConfigured: boolean;
};

/** One recorded usage sample. Local evidence; nothing is uploaded. */
export type LicenseUsageSample = {
  id: string;
  serversUsed: number;
  serversLimit: number;
  agentCount: number;
  envCount: number;
  userCount: number;
  state: LicenseState;
  overSince?: string;
  sampledAt: string;
};

/** The whole activation form: one key, pasted. */
export type LicenseActivateInput = {
  key: string;
};

/** `-1` is the seat value meaning "no ceiling". */
export const UNLIMITED_SERVERS = -1;
