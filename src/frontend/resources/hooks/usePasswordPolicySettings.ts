// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

type SettingRow = {
  id: string;
  key: string;
  value: string;
  category: string;
  updatedAt: string;
};

/**
 * Password policy settings. Mirrors the runtime-configurable knobs
 * in `core/auth/password_policy.go` (PasswordPolicy). Each value is
 * stored as a settings row keyed by the same string the backend
 * reads, so admins can tune the policy without restarting.
 */
export type PolicyForm = {
  password_min_length: number;
  password_require_upper: boolean;
  password_require_lower: boolean;
  password_require_digit: boolean;
  password_require_symbol: boolean;
  password_max_age_days: number;
  password_history_size: number;
};

const POLICY_KEYS: Array<keyof PolicyForm> = [
  'password_min_length',
  'password_require_upper',
  'password_require_lower',
  'password_require_digit',
  'password_require_symbol',
  'password_max_age_days',
  'password_history_size',
];

export const POLICY_DEFAULTS: PolicyForm = {
  password_min_length: 12,
  password_require_upper: true,
  password_require_lower: true,
  password_require_digit: true,
  password_require_symbol: true,
  password_max_age_days: 0,
  password_history_size: 5,
};

/** Bounds the form validates against before the request is sent. */
export const MIN_LENGTH_BOUND = 8;
export const MAX_AGE_BOUND = 365;
export const HISTORY_BOUND = 50;

export function usePasswordPolicy() {
  return useQuery({
    queryKey: ['settings', ...POLICY_KEYS],
    queryFn: async () => {
      const result: Record<string, SettingRow | null> = {};
      await Promise.all(
        POLICY_KEYS.map(async (key) => {
          const res = await api.get<SettingRow>(`/settings/${key}`);
          if (res.success && res.data) {
            result[key] = res.data;
          } else {
            result[key] = null;
          }
        }),
      );
      const out: PolicyForm = { ...POLICY_DEFAULTS };
      if (result.password_min_length) {
        const n = parseInt(result.password_min_length.value, 10);
        if (Number.isFinite(n) && n >= 0) out.password_min_length = n;
      }
      if (result.password_require_upper) {
        out.password_require_upper = result.password_require_upper.value === 'true';
      }
      if (result.password_require_lower) {
        out.password_require_lower = result.password_require_lower.value === 'true';
      }
      if (result.password_require_digit) {
        out.password_require_digit = result.password_require_digit.value === 'true';
      }
      if (result.password_require_symbol) {
        out.password_require_symbol = result.password_require_symbol.value === 'true';
      }
      if (result.password_max_age_days) {
        const n = parseInt(result.password_max_age_days.value, 10);
        if (Number.isFinite(n) && n >= 0) out.password_max_age_days = n;
      }
      if (result.password_history_size) {
        const n = parseInt(result.password_history_size.value, 10);
        if (Number.isFinite(n) && n >= 0) out.password_history_size = n;
      }
      return out;
    },
  });
}

export function useSavePasswordPolicy() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: async (form: PolicyForm) => {
      for (const key of POLICY_KEYS) {
        const value = typeof form[key] === 'boolean' ? String(form[key]) : String(form[key]);
        await api
          .put(`/settings/${key}`, {
            key,
            value,
            category: 'security',
          })
          .then(assertSuccess);
      }
    },
    meta: { success: t('toast.settingsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
      // Every password field in the app pre-validates against
      // GET /auth/password-policy, which is cached for five minutes.
      // Without this the admin who just tightened the policy keeps
      // seeing the old requirements on the create-user dialog.
      void queryClient.invalidateQueries({ queryKey: ['auth', 'password-policy'] });
    },
  });
}
