// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/server';
import { SETUP_TOKEN_HEADER, setupRefusal, setupTokenHeaders, writeSetupToken } from './setupToken';
import { useAuth } from './useAuth';

const INVALID =
  'The setup token is missing or out of date. Copy the current one from the McHarbor log; it changes every time McHarbor restarts.';

describe('first-run setup token', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('sends the token the operator entered, and nothing when there is none', () => {
    expect(setupTokenHeaders()).toEqual({});
    writeSetupToken('  0123abcd  ');
    expect(setupTokenHeaders()).toEqual({ [SETUP_TOKEN_HEADER]: '0123abcd' });
  });

  it('turns a setup_token refusal into the message that says where the token is', () => {
    expect(() => setupRefusal({ success: false, code: 'err.forbidden', data: { reason: 'setup_token' } })).toThrow(
      INVALID,
    );
  });

  it('creates the first admin only with the header, and explains a stale token', async () => {
    server.use(
      http.post('*/api/auth/setup', ({ request }) =>
        request.headers.get(SETUP_TOKEN_HEADER) === 'boot-token'
          ? HttpResponse.json({ success: true, data: { id: 'u1', username: 'admin', role: 'admin' } }, { status: 201 })
          : HttpResponse.json(
              { success: false, code: 'err.forbidden', data: { reason: 'setup_token' } },
              { status: 403 },
            ),
      ),
    );

    writeSetupToken('stale');
    expect(await useAuth.getState().setup('admin', 'Str0ng-passw0rd!')).toEqual({ success: false, error: INVALID });

    writeSetupToken('boot-token');
    expect(await useAuth.getState().setup('admin', 'Str0ng-passw0rd!')).toEqual({ success: true });
  });
});
