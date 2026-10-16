// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { SECRETS_QUERY_KEY } from '@resources/hooks/useSecretsList';
import { assertSuccess } from '@resources/utils/api-mutation';
import type {
  ApplySecretInput,
  CreateSecretInput,
  PatchSecretInput,
  RotateSecretInput,
  Secret,
  SecretApplyResult,
  SecretImpact,
  SecretReference,
  SecretValueResponse,
} from '@core/types/secrets';

const SECRETS_KEY = SECRETS_QUERY_KEY;

export { useSecretsList as useSecrets } from '@resources/hooks/useSecretsList';

/** Lists what uses a secret, for the delete dialog and the rotation banner. */
export function useSecretReferences(id: string | null) {
  return useQuery({
    queryKey: [SECRETS_KEY, 'references', id],
    queryFn: () => api.get<SecretReference[]>(`/secrets/${id}/references`).then((r) => r.data ?? []),
    enabled: Boolean(id),
  });
}

export function useSecretImpact(id: string | null) {
  return useQuery({
    queryKey: [SECRETS_KEY, 'impact', id],
    queryFn: () => api.get<SecretImpact>(`/secrets/${id}/impact`).then((r) => r.data ?? null),
    enabled: Boolean(id),
  });
}

export function useApplySecret() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, resources }: ApplySecretInput) =>
      api.post<SecretApplyResult[]>(`/secrets/${id}/apply`, { resources: resources ?? [] }).then((r) => {
        assertSuccess(r);
        return r.data ?? [];
      }),
    meta: { success: t('toast.applied') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}

export function usePruneSecretReferences() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, resources }: ApplySecretInput) =>
      api.post<{ pruned: number }>(`/secrets/${id}/references/prune`, { resources: resources ?? [] }).then((r) => {
        assertSuccess(r);
        return r.data?.pruned ?? 0;
      }),
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}

export function useCreateSecret() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (input: CreateSecretInput) =>
      api.post<SecretValueResponse | Secret>(`/secrets${envId ? `?env=${envId}` : ''}`, input).then((r) => {
        assertSuccess(r);
        return r.data;
      }),
    meta: { success: t('toast.created') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}

export function useUpdateSecret() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ id, ...body }: PatchSecretInput) =>
      api.patch<Secret>(`/secrets/${id}${envId ? `?env=${envId}` : ''}`, body).then((r) => {
        assertSuccess(r);
        return r.data;
      }),
    meta: { success: t('toast.updated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}

export function useRotateSecret() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ id, ...body }: RotateSecretInput) =>
      api.put<SecretValueResponse | Secret>(`/secrets/${id}${envId ? `?env=${envId}` : ''}`, body).then((r) => {
        assertSuccess(r);
        return r.data;
      }),
    meta: { success: t('toast.rotated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}

export function useDeleteSecret() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, force }: { id: string; force?: boolean }) =>
      api.del(`/secrets/${id}`, force ? { force: 'true' } : undefined).then(assertSuccess),
    meta: { success: t('toast.deleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}

/** Mints a value without storing it, so the form can show one before saving. */
export function useGenerateSecretValue() {
  return useMutation({
    mutationFn: (byteLength?: number) =>
      api.post<SecretValueResponse>('/secrets/generate', { length: byteLength ?? 0 }).then((r) => {
        assertSuccess(r);
        return r.data?.value ?? '';
      }),
  });
}

/**
 * Opens a five-minute step-up ("sudo") window on the caller's session.
 *
 * The reveal endpoint requires **both** an approved elevation for
 * `secrets.reveal` and a live sudo window. The elevation says an approver
 * agreed this person may reveal; the window says the person at the keyboard
 * right now is that person. An elevation alone does not survive a stolen
 * session cookie.
 *
 * The password or TOTP code is sent once and kept nowhere — not in the Query
 * cache, not in a store, not in component state past the submit that spends it.
 */
export function useStepUp() {
  return useMutation({
    mutationFn: ({ password, totpCode }: { password?: string; totpCode?: string }) =>
      api
        .post<{ expiresAt: string; expiresIn: number }>('/auth/sudo', {
          scope: 'secrets.reveal',
          password,
          totpCode,
        })
        .then((r) => {
          assertSuccess(r);
          return r.data;
        }),
  });
}

/**
 * Reveals a secret's plaintext value.
 *
 * This is a **mutation**, not a query, and that is the whole point. A query
 * would put the credential in the TanStack cache, where it would survive
 * navigation, show up in any devtools state dump, and outlive the dialog that
 * asked for it. A mutation returns the value to the caller once and keeps
 * nothing — the dialog holds it in component state for the lifetime of the
 * dialog and no longer.
 *
 * It is also a POST server-side, for the same reason: a GET would land in
 * browser history, proxy logs and referrer headers.
 *
 * Every call writes an audit entry and counts against the caller's hourly
 * rate limit, so nothing here should be called speculatively or on mount.
 */
export function useRevealSecret() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post<SecretValueResponse>(`/secrets/${id}/reveal`, { reason }).then((r) => {
        assertSuccess(r);
        return r.data?.value ?? '';
      }),
    onSuccess: () => {
      // last_revealed_at moved; the list column should say so. The revealed
      // value itself is deliberately not written anywhere.
      void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] });
    },
  });
}
