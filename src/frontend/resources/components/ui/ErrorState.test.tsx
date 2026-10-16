// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { assertSuccess } from '@resources/utils/api-mutation';

function failure(): unknown {
  try {
    assertSuccess({
      success: false,
      error: 'Failed to list containers',
      detail: 'dial tcp 10.0.0.5:2376: connection refused',
      requestId: 'req-42',
    });
  } catch (err) {
    return err;
  }
  return undefined;
}

describe('ErrorState', () => {
  it('shows the server message and cause when no description is given', () => {
    render(<ErrorState error={failure()} />);
    expect(
      screen.getByText('Failed to list containers: dial tcp 10.0.0.5:2376: connection refused'),
    ).toBeInTheDocument();
  });

  it('keeps the server message visible under a caller-supplied description', () => {
    render(<ErrorState error={failure()} description="The container list could not be loaded." />);
    expect(screen.getByText('The container list could not be loaded.')).toBeInTheDocument();
    expect(screen.getByText(/connection refused/)).toBeInTheDocument();
  });

  it('offers the request ID as the error ID', () => {
    render(<ErrorState error={failure()} />);
    expect(screen.getByText(/req-42/)).toBeInTheDocument();
  });
});
