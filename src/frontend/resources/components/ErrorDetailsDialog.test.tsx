// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { Toaster, toast } from 'sonner';
import { afterEach, describe, expect, it } from 'vitest';
import { ErrorDetailsDialog } from '@resources/components/ErrorDetailsDialog';
import { useErrorDetails } from '@resources/stores/errorDetails';
import { assertSuccess } from '@resources/utils/api-mutation';
import { toastError } from '@resources/utils/toast';

const cause =
  'Error response from daemon: driver failed programming external connectivity on endpoint web: ' +
  'Bind for 0.0.0.0:8080 failed: port is already allocated';

function failure(): unknown {
  try {
    assertSuccess({
      success: false,
      error: 'Failed to start container',
      code: 'err.container.start_failed',
      detail: cause,
      requestId: 'cs1abc',
    });
  } catch (err) {
    return err;
  }
  return undefined;
}

describe('error toast details', () => {
  afterEach(() =>
    act(() => {
      useErrorDetails.getState().close();
      toast.dismiss();
    }),
  );

  it('shows the operation as title and the server message with its cause beneath', async () => {
    render(<Toaster />);
    act(() => toastError(failure(), 'Start container'));
    expect(await screen.findByText('Start container')).toBeInTheDocument();
    expect(screen.getByText(/Failed to start container — .*port is already allocated/)).toBeInTheDocument();
  });

  it('opens the full record from the Details action, untruncated, with code and request ID', async () => {
    render(
      <>
        <Toaster />
        <ErrorDetailsDialog />
      </>,
    );
    act(() => toastError(failure()));
    // sonner's pointer handlers need setPointerCapture, which jsdom lacks; a click is enough here.
    fireEvent.click(await screen.findByRole('button', { name: 'Details' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(cause)).toBeInTheDocument();
    expect(within(dialog).getByText('err.container.start_failed')).toBeInTheDocument();
    expect(within(dialog).getByText('cs1abc')).toBeInTheDocument();
  });

  it('offers no Details action when there is nothing beyond the message', async () => {
    render(<Toaster />);
    act(() => toastError(new Error('Clipboard unavailable')));
    expect(await screen.findByText('Clipboard unavailable')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Details' })).not.toBeInTheDocument();
  });
});
