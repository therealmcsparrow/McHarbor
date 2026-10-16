// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type ApiResponse } from '@core/api/client';
import { ApprovalRequiredError, approvalHold, describeApiError } from '@resources/utils/api-mutation';
import { runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';
import { appToast } from '@resources/utils/toast';
import { isDemoConfigField } from '@resources/constants/demoConfig';
import type {
  DemoAction,
  DemoChangeRequest,
  DemoConfigField,
  DemoOutcome,
  DemoRefusal,
  DemoStatus,
} from '@resources/types/demo';

export const DEMO_STATUS_KEY = ['demo', 'status'] as const;

const REFUSALS: readonly DemoRefusal[] = ['already_active', 'not_active', 'clustered', 'busy'];

export class DemoConfigRefusal extends Error {
  field: DemoConfigField;
  min: number;
  max: number;

  constructor(message: string, field: DemoConfigField, min: number, max: number) {
    super(message);
    this.name = 'DemoConfigRefusal';
    this.field = field;
    this.min = min;
    this.max = max;
  }
}

export function demoRefusalOf(res: Pick<ApiResponse, 'data'>): DemoRefusal | undefined {
  const data = res.data;
  if (!data || typeof data !== 'object') return undefined;
  const reason = Reflect.get(data, 'reason');
  return REFUSALS.find((candidate) => candidate === reason);
}

export function demoConfigRefusalOf(
  res: Pick<ApiResponse, 'data'>,
): { field: DemoConfigField; min: number; max: number } | undefined {
  const data = res.data;
  if (!data || typeof data !== 'object') return undefined;
  if (Reflect.get(data, 'reason') !== 'invalid_config') return undefined;
  const field: unknown = Reflect.get(data, 'field');
  const min: unknown = Reflect.get(data, 'min');
  const max: unknown = Reflect.get(data, 'max');
  if (!isDemoConfigField(field) || typeof min !== 'number' || typeof max !== 'number') return undefined;
  return { field, min, max };
}

export function useDemoStatus() {
  return useQuery({
    queryKey: DEMO_STATUS_KEY,
    queryFn: async () => {
      const res = await api.get<DemoStatus>('/demo/status');
      if (!res.success || !res.data) throw new Error(describeApiError(res));
      return res.data;
    },
    staleTime: 30_000,
    refetchOnWindowFocus: true,
  });
}

export function useDemoAction(action: DemoAction) {
  const { t } = useTranslation('demo');
  const queryClient = useQueryClient();

  function refusalMessage(reason: DemoRefusal): string {
    switch (reason) {
      case 'already_active':
        return t('reason.already_active');
      case 'not_active':
        return t('reason.not_active');
      case 'clustered':
        return t('reason.clustered');
      case 'busy':
        return t('reason.busy');
    }
  }

  function successMessage(outcome: DemoOutcome): string {
    switch (action) {
      case 'enter':
        return t('toast.enterSuccess');
      case 'exit':
        return t('toast.exitSuccess');
      case 'reset':
        return outcome.restartScheduled ? t('toast.resetRestart') : t('toast.resetSuccess');
    }
  }

  return useMutation({
    mutationFn: async (request: DemoChangeRequest | undefined): Promise<DemoOutcome> => {
      const res = await api.post<DemoOutcome>(`/demo/${action}`, request);
      if (!res.success) {
        const hold = approvalHold(res);
        if (hold) throw new ApprovalRequiredError(hold);
        const invalid = demoConfigRefusalOf(res);
        if (invalid) {
          throw new DemoConfigRefusal(t('config.refused'), invalid.field, invalid.min, invalid.max);
        }
        const reason = demoRefusalOf(res);
        throw new Error(reason ? refusalMessage(reason) : describeApiError(res));
      }
      const outcome: DemoOutcome = {
        restartScheduled: res.data?.restartScheduled === true,
        restartRequired: res.data?.restartRequired === true,
      };
      if (outcome.restartScheduled) {
        await runWithMcHarborRestart(() => Promise.resolve(outcome));
      }
      return outcome;
    },
    onSuccess: (outcome) => {
      void queryClient.invalidateQueries({ queryKey: DEMO_STATUS_KEY });
      if (outcome.restartRequired && !outcome.restartScheduled) {
        appToast.warning(t('toast.restartRequired'));
        return;
      }
      appToast.success(successMessage(outcome));
    },
  });
}
