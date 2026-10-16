// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTestPolicy, type PolicyTestAttributes } from '@resources/hooks/usePolicies';

export type PolicyTestResult = {
  matched: boolean;
  reason: string;
  error?: string;
};

// The attribute document the tester seeds its editor with. Values are
// deliberately obvious placeholders: the point is to show the shape a policy
// expression can read, not to look like a real user.
export const EMPTY_ATTRS: PolicyTestAttributes = {
  user: {
    id: 'demo-user',
    username: 'demo',
    email: 'demo@example.com',
    roles: ['operator'],
    groups: [],
    labels: {},
  },
  env: { id: 'env-local', name: 'local', type: 'docker' },
  resource: {
    type: '',
    id: 'res-1',
    owner: 'demo-user',
    image: '',
    state: 'running',
    labels: {},
  },
  now: { year: 2026, month: 1, day: 1, hour: 12, weekday: 1 },
};

/**
 * State behind PolicyTesterDialog: the editable attribute JSON, its parse
 * state, and the run that sends it to the sandbox.
 *
 * The parse error is derived from the raw text rather than stored, so the two
 * can never disagree — the previous version set it from inside a useMemo,
 * which is a state update during render.
 */
export function usePolicyTester(policyId: string, policyResourceType: string, isOpen: boolean) {
  const [rawJson, setRawJson] = useState(() => JSON.stringify(EMPTY_ATTRS, null, 2));
  const [result, setResult] = useState<PolicyTestResult | null>(null);

  const test = useTestPolicy(policyId);

  // Reset every time the dialog re-opens or the policy under test changes.
  const resetKey = isOpen ? policyResourceType : null;
  const [appliedResetKey, setAppliedResetKey] = useState<string | null>(null);
  if (appliedResetKey !== resetKey) {
    setAppliedResetKey(resetKey);
    if (isOpen) {
      const seed: PolicyTestAttributes = {
        ...EMPTY_ATTRS,
        resource: { ...EMPTY_ATTRS.resource, type: policyResourceType },
      };
      setRawJson(JSON.stringify(seed, null, 2));
      setResult(null);
    }
  }

  const { parsed, parseError } = useMemo(() => {
    try {
      return { parsed: JSON.parse(rawJson) as PolicyTestAttributes, parseError: null };
    } catch (e) {
      return { parsed: null, parseError: (e as Error).message };
    }
  }, [rawJson]);

  const run = () => {
    if (!parsed) return;
    test.mutate(parsed, { onSuccess: (res) => setResult(res) });
  };

  return { rawJson, setRawJson, parsed, parseError, result, test, run };
}
