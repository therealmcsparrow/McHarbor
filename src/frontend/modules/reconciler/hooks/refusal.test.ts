// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeAll, describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';
import { i18n, initTestI18n } from '../../../test/i18n';
import { ReconcilerRefusal, refusalText } from './refusal';

let t: TFunction<'reconciler'>;
beforeAll(async () => {
  await initTestI18n();
  t = i18n.getFixedT('en', 'reconciler') as TFunction<'reconciler'>;
});

describe('refusalText', () => {
  it('names every validation reason the backend can send, instead of the generic refusal', () => {
    const reasons: Record<string, string> = {
      confirm_passes_range: 'Confirm passes must be between 1 and 10.',
      actions_per_pass_range: 'Actions per pass must be between 1 and 25.',
      cooldown_range: 'The cooldown must be between 0 and 86400 seconds.',
      failure_threshold_range: 'The failure threshold must be between 1 and 20.',
      source_fields_required: 'A name, a repository and an environment are required.',
      invalid_branch: 'That branch name is not valid.',
      path_escapes_repo: 'The path must stay inside the repository.',
      invalid: 'The request was not valid.',
    };
    for (const [reason, text] of Object.entries(reasons)) {
      expect(refusalText(new ReconcilerRefusal(reason, 'Invalid request'), t)).toBe(text);
    }
  });

  it('keeps the message of an error that carries no reason', () => {
    expect(refusalText(new Error('The server is unreachable'), t)).toBe('The server is unreachable');
  });
});
