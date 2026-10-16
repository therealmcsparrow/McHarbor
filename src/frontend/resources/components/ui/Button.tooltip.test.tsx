// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactElement } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { Button } from './Button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './Tooltip';

// Radix opens a tooltip on focus as well as hover, and focus is the one jsdom
// drives reliably — the same reason Button.permission.test.tsx tabs to it rather
// than hovering.
function withProvider(ui: ReactElement) {
  return <TooltipProvider>{ui}</TooltipProvider>;
}

describe('Button tooltips on icon-only buttons', () => {
  beforeEach(() => {
    resetPermissions();
  });

  it('shows the aria-label as a tooltip, so a pointer user learns the name too', async () => {
    const user = userEvent.setup();
    render(withProvider(<Button size="icon" aria-label="Remove volume" />));

    await user.tab();

    expect(await screen.findAllByText('Remove volume')).not.toHaveLength(0);
  });

  it('prefers an explicit tooltip over the aria-label', async () => {
    const user = userEvent.setup();
    render(withProvider(<Button size="icon-sm" aria-label="Prune" tooltip="Remove every unused image" />));

    await user.tab();

    expect(await screen.findAllByText('Remove every unused image')).not.toHaveLength(0);
  });

  it('adds no tooltip to a button with visible text', async () => {
    const user = userEvent.setup();
    render(withProvider(<Button aria-label="Save changes">Save</Button>));

    await user.tab();

    // The label is already on screen; a tooltip repeating it is noise. Only the
    // button's own text node should exist.
    expect(screen.queryAllByText('Save changes')).toHaveLength(0);
  });

  it('honours tooltip={false} as an opt-out', async () => {
    const user = userEvent.setup();
    render(withProvider(<Button size="icon" aria-label="Drag handle" tooltip={false} />));

    await user.tab();

    expect(screen.queryAllByText('Drag handle')).toHaveLength(0);
  });

  it('does not wrap itself when the caller already wrapped it', async () => {
    // A Radix tooltip nested inside another one's trigger opens neither, so the
    // call sites that compose their own tooltip must keep working unchanged.
    const user = userEvent.setup();
    render(
      withProvider(
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="icon" aria-label="Refresh" />
          </TooltipTrigger>
          <TooltipContent>Reload the table</TooltipContent>
        </Tooltip>,
      ),
    );

    await user.tab();

    expect(await screen.findAllByText('Reload the table')).not.toHaveLength(0);
    // The caller's text, not the aria-label, and exactly one tooltip.
    expect(screen.queryAllByText('Refresh')).toHaveLength(0);
  });

  it('still explains a denied permission rather than showing the aria-label', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(withProvider(<Button size="icon" aria-label="Remove volume" permission="volumes.delete" />));

    expect(screen.getByRole('button', { name: 'Remove volume' })).toBeDisabled();

    await user.tab();

    // The gate's message names the missing key, which is the more useful thing to
    // say, and must not be displaced by the button's own name.
    expect(await screen.findAllByText('Requires the volumes.delete permission')).not.toHaveLength(0);
  });

  it('leaves an icon button with no aria-label alone', async () => {
    // Nothing to say, and inventing text would be worse than silence. The missing
    // label is its own defect, caught by the accessibility suite.
    const user = userEvent.setup();
    render(withProvider(<Button size="icon" data-testid="nameless" />));

    await user.tab();

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });
});
