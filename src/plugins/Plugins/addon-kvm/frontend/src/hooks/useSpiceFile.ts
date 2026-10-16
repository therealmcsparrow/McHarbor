// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { api, type ApiResponse } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * Download a remote-viewer (.vv) file for a guest.
 *
 * A mutation, not a link: the file holds a one-time password, so it is fetched
 * only when asked for, and a refusal arrives as the server's envelope, which
 * `assertSuccess` turns into the translated message the mutation cache shows.
 */
export function useSpiceFile() {
  return useMutation({
    mutationFn: async (input: { id: string; name: string; host?: string }) => {
      const host = input.host?.trim();
      const response = await api.download(`/vms/${input.id}/console.vv`, host ? { host } : undefined);
      if (!response.ok) {
        const envelope: ApiResponse<null> = await response.json().catch(() => ({ success: false }));
        assertSuccess({ ...envelope, success: false });
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${input.name}.vv`;
      link.click();
      // Revoke on the next tick, after the browser has taken the blob.
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
      return null;
    },
  });
}
