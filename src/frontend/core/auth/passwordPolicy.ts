// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Client-side mirror of `core/auth.PasswordPolicy`.
 *
 * The browser copy exists so a user learns a password is unacceptable
 * while typing it, not from a 400 after they hit submit. It is NOT a
 * security boundary — every write path re-validates server-side in
 * `auth.ValidatePassword`. Keep the rules here identical to that
 * function; a client rule the server does not enforce is a lie, and a
 * server rule missing here is a surprise rejection.
 */
export type PasswordPolicy = {
  minLength: number;
  requireUpper: boolean;
  requireLower: boolean;
  requireDigit: boolean;
  requireSymbol: boolean;
  /** Informational only — the browser cannot check password age. */
  maxAgeDays: number;
  /** Informational only — history lives server-side. */
  historySize: number;
};

/**
 * Matches `auth.DefaultPasswordPolicy()`. Used until the policy
 * request resolves and as the fallback when it fails, so a network
 * hiccup can only make the client stricter than the server, never
 * looser.
 */
export const DEFAULT_PASSWORD_POLICY: PasswordPolicy = {
  minLength: 12,
  requireUpper: true,
  requireLower: true,
  requireDigit: true,
  requireSymbol: true,
  maxAgeDays: 0,
  historySize: 5,
};

/** The rule identifiers the checklist renders, in display order. */
export type PasswordRuleId = 'minLength' | 'requireUpper' | 'requireLower' | 'requireDigit' | 'requireSymbol';

export type PasswordRuleResult = {
  id: PasswordRuleId;
  /** Interpolation value for the i18n label ("At least {{count}} characters"). */
  count?: number;
  met: boolean;
};

export type PasswordEvaluation = {
  /** Only the rules the active policy actually requires. */
  rules: PasswordRuleResult[];
  /** True when every required rule passes. */
  valid: boolean;
};

// Mirrors the Go side: unicode.IsPunct(r) || unicode.IsSymbol(r).
// JS has no unicode.IsPunct, so we use the Unicode property escapes
// for the same two general categories (P* and S*).
const SYMBOL_RE = /[\p{P}\p{S}]/u;
const UPPER_RE = /\p{Lu}/u;
const LOWER_RE = /\p{Ll}/u;
const DIGIT_RE = /\p{Nd}/u;

/**
 * Evaluates a candidate against the policy and returns one result per
 * required rule. Rules the policy does not require are omitted so the
 * checklist never shows a box the user cannot fail.
 */
export function evaluatePassword(password: string, policy: PasswordPolicy): PasswordEvaluation {
  const rules: PasswordRuleResult[] = [];

  if (policy.minLength > 0) {
    rules.push({
      id: 'minLength',
      count: policy.minLength,
      // The Go check is `len(p) < MinLength` — bytes, not runes. For
      // ASCII the two agree; for multi-byte input the byte count is
      // always >= the code-unit count, so counting UTF-16 units here
      // stays conservative (the client never accepts what the server
      // would reject on length).
      met: password.length >= policy.minLength,
    });
  }
  if (policy.requireUpper) {
    rules.push({ id: 'requireUpper', met: UPPER_RE.test(password) });
  }
  if (policy.requireLower) {
    rules.push({ id: 'requireLower', met: LOWER_RE.test(password) });
  }
  if (policy.requireDigit) {
    rules.push({ id: 'requireDigit', met: DIGIT_RE.test(password) });
  }
  if (policy.requireSymbol) {
    rules.push({ id: 'requireSymbol', met: SYMBOL_RE.test(password) });
  }

  return { rules, valid: rules.every((rule) => rule.met) };
}

/** Convenience wrapper for submit-button gating. */
export function isPasswordValid(password: string, policy: PasswordPolicy): boolean {
  return evaluatePassword(password, policy).valid;
}
