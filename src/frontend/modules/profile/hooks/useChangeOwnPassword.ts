// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useAuth } from '@core/auth/useAuth';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * useChangeOwnPassword changes the password of the currently
 * authenticated user. The backend route is `PUT /users/{id}/password`
 * and requires the current password when the actor is the same
 * user. Admin resets go through a different endpoint that
 * doesn't require currentPassword.
 */
export function useChangeOwnPassword() {
  return useMutation({
    mutationFn: async (input: { currentPassword: string; newPassword: string }) => {
      // The current user id is read from the auth store at call
      // time; we don't want this hook to capture it in closure.
      const userId = useAuth.getState().user?.id;
      if (!userId) throw new Error('not authenticated');
      await api.put(`/users/${userId}/password`, input).then(assertSuccess);
    },
  });
}
