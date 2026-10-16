// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { ApiResponse } from '@core/api/client';
import { recordStep, StepDeniedError } from './useRunbooks';
import type { RunbookStepResult } from '@resources/types/runbooks';

function refusal(code: string, data: unknown, error = 'server said no'): ApiResponse<RunbookStepResult> {
  return { success: false, code, error, data } as ApiResponse<RunbookStepResult>;
}

describe('recordStep', () => {
  it('names the missing permission when the step is forbidden', () => {
    const run = () => recordStep(refusal('err.runbook.step_forbidden', { permission: 'containers.manage' }));
    expect(run).toThrow(StepDeniedError);
    expect(run).toThrow(/containers\.manage/);
  });

  it('survives a forbidden refusal whose payload carries no permission', () => {
    expect(() => recordStep(refusal('err.runbook.step_forbidden', undefined))).toThrow(StepDeniedError);
  });

  it('surfaces the step command output rather than the envelope message when the step failed', () => {
    expect(() => recordStep(refusal('err.runbook.step_failed', { lastError: '  exit 3: disk full  ' }))).toThrow(
      /^exit 3: disk full$/,
    );
  });

  it('falls back to the envelope message when a failed step reports no output', () => {
    expect(() => recordStep(refusal('err.runbook.step_failed', { lastError: 7 }, 'the step failed'))).toThrow(
      'the step failed',
    );
  });

  it('returns the recorded step on success', () => {
    const data = { id: 'step-1' } as unknown as RunbookStepResult;
    expect(recordStep({ success: true, data })).toBe(data);
  });
});
