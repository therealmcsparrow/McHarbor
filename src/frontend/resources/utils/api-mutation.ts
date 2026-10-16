// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n, { type i18n as I18n } from 'i18next';

import { envelopeStatus, type ApiResponse } from '@core/api/client';

export type MutationMeta = {
  success?: string | ((data: unknown, variables: unknown) => string);
  error?: string | ((err: Error) => string);
};

type ErrorEnvelope = Pick<ApiResponse, 'error' | 'message' | 'code' | 'data' | 'detail' | 'requestId'>;

/**
 * Everything known about one failure, carried as plain data so it survives the
 * plugin bundle boundary, where the host cannot `instanceof` a plugin's ApiError.
 */
export type ErrorDetails = {
  summary: string;
  detail?: string;
  code?: string;
  requestId?: string;
  status?: number;
  operation?: string;
  at: string;
};

export const APPROVAL_REQUIRED = 'approval_required';

export type ApprovalHold = {
  reason: typeof APPROVAL_REQUIRED;
  approvalId: string;
  category?: string;
  requiredApprovers?: number;
  expiresAt?: string;
};

export class ApprovalRequiredError extends Error {
  readonly approvalId: string;
  readonly hold: ApprovalHold;

  constructor(hold: ApprovalHold) {
    super(describeApprovalHold(hold));
    this.name = 'ApprovalRequiredError';
    this.approvalId = hold.approvalId;
    this.hold = hold;
  }
}

export function approvalHold(res: Pick<ApiResponse, 'data'>): ApprovalHold | undefined {
  const data = res.data as Partial<ApprovalHold> | undefined;
  if (data && typeof data === 'object' && data.reason === APPROVAL_REQUIRED && typeof data.approvalId === 'string') {
    return data as ApprovalHold;
  }
  return undefined;
}

export function isApprovalRequired(error: unknown): error is ApprovalRequiredError {
  return error instanceof ApprovalRequiredError;
}

function describeApprovalHold(hold: ApprovalHold): string {
  return translator().t('approvalHeld.sent', { ns: 'common', id: hold.approvalId });
}

export function describeApiError(res: ErrorEnvelope): string {
  const hold = approvalHold(res);
  if (hold) return describeApprovalHold(hold);
  return withDetail(summarizeApiError(res), res.detail);
}

/** The summary line alone: the translated code, or the server's own message. */
export function summarizeApiError(res: ErrorEnvelope): string {
  const resolved = translateErrorCode(res.code);
  const detail = res.error?.trim();
  if (resolved) {
    return detail && detail !== resolved ? `${resolved} ${detail}` : resolved;
  }
  if (detail) return detail;
  if (res.message) return res.message;
  const fallback: string = translator().t('errors.operationFailed', { ns: 'common' });
  return fallback && fallback !== 'errors.operationFailed' ? fallback : 'Operation failed';
}

/**
 * The live i18next instance, which inside a plugin bundle is the host's.
 *
 * A plugin compiles its own copy of i18next and never initialises it — it binds
 * the host's instance through `window.McharborHost.i18n` instead. The imported
 * singleton is therefore empty in that context, and every lookup through it
 * returns nothing, which is how a resolved error message became an empty toast.
 */
function translator(): I18n {
  const host = (window as Window & { McharborHost?: { i18n?: I18n } }).McharborHost?.i18n;
  return host ?? i18n;
}

function withDetail(summary: string, detail: string | undefined): string {
  const cause = detail?.trim();
  if (!cause || summary.includes(cause)) return summary;
  return /[.!?:]$/.test(summary) ? `${summary} ${cause}` : `${summary}: ${cause}`;
}

/** The translated fallback for a failure that carried no message of its own. */
export function operationFailedMessage(): string {
  return translator().t('errors.operationFailed', { ns: 'common' });
}

export function translateErrorCode(code: string | undefined): string | undefined {
  if (!code) return undefined;

  const t = translator();
  const namespace = code.split('.')[1];
  const attempts = namespace ? [{ ns: namespace }, undefined] : [undefined];

  for (const options of attempts) {
    const translated: string = options ? t.t(code, options) : t.t(code);
    if (translated && translated !== code) return translated;
  }
  return undefined;
}

/** ApiError is what assertSuccess throws, carrying the HTTP status so a retry policy can tell a 4xx from a flaky 5xx. */
export class ApiError extends Error {
  readonly status: number | undefined;
  readonly details: ErrorDetails;
  /** Read by `<ErrorState>`, which offers it for copying. */
  readonly errorId: string | undefined;

  constructor(message: string, httpStatus?: number, details?: Omit<ErrorDetails, 'at'>) {
    super(message);
    this.name = 'ApiError';
    this.status = httpStatus;
    this.details = { summary: message, status: httpStatus, ...details, at: new Date().toISOString() };
    this.errorId = this.details.requestId;
  }
}

/** apiErrorOf builds the ApiError a failed envelope stands for, outside assertSuccess's throw. */
export function apiErrorOf(res: ApiResponse<unknown>): ApiError {
  const httpStatus = envelopeStatus(res);
  return new ApiError(describeApiError(res), httpStatus, {
    summary: summarizeApiError(res),
    detail: res.detail?.trim() || undefined,
    code: res.code,
    requestId: res.requestId,
    status: httpStatus,
  });
}

/**
 * errorDetailsOf reads the details off any thrown value. It duck-types rather
 * than using instanceof, because a plugin's ApiError is a different class.
 */
export function errorDetailsOf(error: unknown, operation?: string): ErrorDetails {
  const at = new Date().toISOString();
  const carried = typeof error === 'object' && error !== null ? Reflect.get(error, 'details') : undefined;
  if (carried && typeof carried === 'object' && typeof Reflect.get(carried, 'summary') === 'string') {
    return { ...(carried as ErrorDetails), operation: operation ?? (carried as ErrorDetails).operation };
  }
  if (error instanceof Error) return { summary: error.message || operationFailedMessage(), operation, at };
  return { summary: typeof error === 'string' && error ? error : operationFailedMessage(), operation, at };
}

/** isClientError reports a 4xx failure, which repeating the same request cannot fix. */
export function isClientError(error: unknown): boolean {
  return error instanceof ApiError && error.status !== undefined && error.status >= 400 && error.status < 500;
}

export function assertSuccess<T>(res: ApiResponse<T>): T {
  if (!res.success) {
    const hold = approvalHold(res);
    if (hold) throw new ApprovalRequiredError(hold);
    throw apiErrorOf(res);
  }
  return res.data as T;
}
