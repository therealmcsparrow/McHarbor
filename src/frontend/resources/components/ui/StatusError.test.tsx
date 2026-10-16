// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { StatusError } from '@resources/components/ui/StatusError';

const longError =
  'qemu-img: Could not open /var/lib/mcharbor/imports/disk.vmdk: VMDK version 3 must be read only; ' +
  'the descriptor references an extent that is missing from the upload, so conversion cannot start.';

describe('StatusError', () => {
  it.each([undefined, null, '', '   '])('renders nothing for %p', (error) => {
    const { container } = render(<StatusError error={error} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows an icon button, not the message, until opened', () => {
    render(<StatusError error={longError} />);
    expect(screen.getByRole('button', { name: 'View the error' })).toBeInTheDocument();
    expect(screen.queryByText(longError)).not.toBeInTheDocument();
  });

  it('opens the full, untruncated message with the default title', async () => {
    const user = userEvent.setup();
    render(<StatusError error={longError} />);
    await user.click(screen.getByRole('button', { name: 'View the error' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Error details')).toBeInTheDocument();
    expect(within(dialog).getByText(longError)).toBeInTheDocument();
  });

  it('uses a caller-supplied title and description', async () => {
    const user = userEvent.setup();
    render(<StatusError error="boom" title="Import failed" description="What qemu-img reported." />);
    await user.click(screen.getByRole('button', { name: 'View the error' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Import failed')).toBeInTheDocument();
    expect(within(dialog).getByText('What qemu-img reported.')).toBeInTheDocument();
  });
});
