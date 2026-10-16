// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, describe, expect, it } from 'vitest';
import {
  ApiError,
  assertSuccess,
  describeApiError,
  errorDetailsOf,
  translateErrorCode,
} from '@resources/utils/api-mutation';
import { i18n } from '../../test/i18n';

describe('error-code mapping', () => {
  afterEach(() => {
    i18n.removeResourceBundle('en', 'testerrors');
  });

  it('returns undefined for a missing or unknown code', () => {
    expect(translateErrorCode(undefined)).toBeUndefined();
    expect(translateErrorCode('err.nothing.here')).toBeUndefined();
  });

  it('resolves a code through the namespace named by its second segment', () => {
    i18n.addResourceBundle('en', 'testerrors', { err: { testerrors: { in_use: 'Still attached.' } } });
    expect(translateErrorCode('err.testerrors.in_use')).toBe('Still attached.');
  });

  it('shows the server message when the code does not resolve', () => {
    expect(describeApiError({ error: 'Volume v1 is used by container web', code: 'err.volume.in_use' })).toBe(
      'Volume v1 is used by container web',
    );
  });

  it('appends the server detail to a resolved code without repeating it', () => {
    i18n.addResourceBundle('en', 'testerrors', { err: { testerrors: { in_use: 'Still attached.' } } });
    expect(describeApiError({ code: 'err.testerrors.in_use', error: 'used by web' })).toBe(
      'Still attached. used by web',
    );
    expect(describeApiError({ code: 'err.testerrors.in_use', error: 'Still attached.' })).toBe('Still attached.');
  });

  it('falls back to message, then to the translated generic failure', () => {
    expect(describeApiError({ message: 'Queued but refused' })).toBe('Queued but refused');
    expect(describeApiError({})).toBe(i18n.t('errors.operationFailed', { ns: 'common' }));
    expect(describeApiError({})).not.toBe('errors.operationFailed');
  });

  it('assertSuccess returns data on success and throws the described error on failure', () => {
    expect(assertSuccess({ success: true, data: 7 })).toBe(7);
    expect(() => assertSuccess({ success: false, error: 'Denied by policy' })).toThrow('Denied by policy');
  });
});

describe('error cause and request ID', () => {
  it('appends the server cause to the summary so every error.message reader shows it', () => {
    expect(describeApiError({ error: 'Failed to start container', detail: 'port is already allocated' })).toBe(
      'Failed to start container: port is already allocated',
    );
    expect(describeApiError({ error: 'Failed to start container.', detail: 'port is already allocated' })).toBe(
      'Failed to start container. port is already allocated',
    );
  });

  it('assertSuccess throws an ApiError carrying code, cause and request ID', () => {
    let thrown: unknown;
    try {
      assertSuccess({
        success: false,
        error: 'Failed to start container',
        code: 'err.x',
        detail: 'port is already allocated',
        requestId: 'req-9',
      });
    } catch (err) {
      thrown = err;
    }
    expect(thrown).toBeInstanceOf(ApiError);
    const details = (thrown as ApiError).details;
    expect(details).toMatchObject({
      summary: 'Failed to start container',
      detail: 'port is already allocated',
      code: 'err.x',
      requestId: 'req-9',
    });
    expect((thrown as ApiError).errorId).toBe('req-9');
  });

  it('errorDetailsOf reads details off a foreign error by shape and adds the operation', () => {
    const foreign = Object.assign(new Error('x'), {
      details: { summary: 'From a plugin', detail: 'cause', at: '2026-10-08T00:00:00Z' },
    });
    expect(errorDetailsOf(foreign, 'Start VM')).toMatchObject({
      summary: 'From a plugin',
      detail: 'cause',
      operation: 'Start VM',
    });
    expect(errorDetailsOf(new Error('plain'))).toMatchObject({ summary: 'plain' });
  });
});
