// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MutationCache } from '@tanstack/react-query';

import { createHostToast } from '@resources/utils/hostToast';
import { errorDetailsOf, isApprovalRequired, type MutationMeta } from '@resources/utils/api-mutation';

/**
 * The mutation-error toast a plugin's own QueryClient needs.
 *
 * The host's global toast hangs off the host's QueryClient, and every plugin
 * bundle builds its own — so without this a rejected request leaves the UI
 * exactly as it was, the operator sees a status code in the browser console and
 * nothing else, and they click the button again. Five plugins shipped in that
 * state.
 *
 * `meta.success` and `meta.error` on a mutation override the defaults, matching
 * the host's own MutationCache so a mutation reads the same either side of the
 * boundary.
 */
export function createPluginMutationCache(tag: string): MutationCache {
  const toast = createHostToast(tag);

  return new MutationCache({
    onSuccess: (data, variables, _context, mutation) => {
      const meta = mutation.options.meta as MutationMeta | undefined;
      if (!meta?.success) return;
      const message = typeof meta.success === 'function' ? meta.success(data, variables) : meta.success;
      if (message) toast.success(message);
    },
    onError: (error, _variables, _context, mutation) => {
      if (isApprovalRequired(error)) {
        toast.info(error.message);
        return;
      }
      // meta.error names the operation; the host's message and cause stay under it.
      const meta = mutation.options.meta as MutationMeta | undefined;
      const operation = typeof meta?.error === 'function' ? meta.error(error as Error) : meta?.error;
      toast.error(operation || errorText(error), errorDetailsOf(error, operation || undefined));
    },
  });
}

/**
 * Keeps the host's own wording when there is one. A plugin failure is usually a
 * sentence about the host — a missing supervisor, an unreachable mirror — and
 * replacing it with a generic string throws away the only part the operator can
 * act on.
 */
function errorText(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return String(error);
}
