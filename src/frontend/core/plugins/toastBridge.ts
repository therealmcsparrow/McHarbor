// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ErrorDetails } from '@resources/utils/api-mutation';
import { appToast, toastErrorDetails } from '@resources/utils/toast';

export function publishToastBridge(): void {
  window.McharborHost = {
    ...window.McharborHost,
    toast: {
      success: (message: string) => appToast.success(message),
      // A rebuilt plugin passes its failure's details; an older bundle passes only the message.
      error: (message: string, details?: ErrorDetails) =>
        toastErrorDetails(details ?? { summary: message, at: new Date().toISOString() }),
      info: (message: string) => appToast.info(message),
      warning: (message: string) => appToast.warning(message),
    },
  };
}
