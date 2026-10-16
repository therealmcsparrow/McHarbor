// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from 'i18next';
import { toast as sonnerToast } from 'sonner';
import { errorDetailsOf, type ErrorDetails } from '@resources/utils/api-mutation';
import { useErrorDetails } from '@resources/stores/errorDetails';

export const appToast = {
  success: sonnerToast.success,
  error: sonnerToast.error,
  info: sonnerToast,
  warning: sonnerToast.warning,
  dismiss: sonnerToast.dismiss,
  promise: sonnerToast.promise,
};

export type AppToast = typeof appToast;

/**
 * toastErrorDetails shows a failure with its cause under the title and a
 * Details action that opens the full record — code, HTTP status, request ID and
 * the untruncated cause — so a long daemon message is never cut off for good.
 */
export function toastErrorDetails(details: ErrorDetails): void {
  const title = details.operation ?? details.summary;
  // A meta.error function often interpolates the message itself; never say it twice.
  const description = [details.summary, details.detail]
    .map((line) => line?.trim() ?? '')
    .filter((line) => line !== '' && !title.includes(line))
    .join(' — ');
  const hasMore = Boolean(details.detail || details.code || details.requestId);
  sonnerToast.error(title, {
    description: description || undefined,
    classNames: { description: 'line-clamp-4 break-words' },
    action: hasMore
      ? {
          label: i18n.t('errorDetails.view', { ns: 'common' }),
          onClick: () => useErrorDetails.getState().show(details),
        }
      : undefined,
  });
}

/** toastError reports any thrown value; `operation` names what was being attempted. */
export function toastError(error: unknown, operation?: string): void {
  toastErrorDetails(errorDetailsOf(error, operation));
}
