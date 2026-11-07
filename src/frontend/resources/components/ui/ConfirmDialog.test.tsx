// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ConfirmDialog } from './ConfirmDialog';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        'actions.cancel': 'Cancel',
        'actions.processing': 'Processing',
        'actions.remove': 'Remove',
      })[key] ?? key,
  }),
}));

describe('ConfirmDialog', () => {
  it('renders translated defaults and emits cancel and confirm actions', () => {
    const onOpenChange = vi.fn();
    const onConfirm = vi.fn();

    render(
      <ConfirmDialog
        open
        onOpenChange={onOpenChange}
        title="Delete volume"
        description="This cannot be undone."
        onConfirm={onConfirm}
      >
        <p>Extra details</p>
      </ConfirmDialog>,
    );

    expect(screen.getByText('Delete volume')).toBeInTheDocument();
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument();
    expect(screen.getByText('Extra details')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('uses the supplied confirm label and disables while loading', () => {
    render(
      <ConfirmDialog
        open
        onOpenChange={vi.fn()}
        title="Restart"
        description="Restart service"
        confirmLabel="Restart now"
        variant="default"
        loading
        onConfirm={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Processing' })).toBeDisabled();
  });

  it('keeps a natively disabled Confirm by default', () => {
    render(<ConfirmDialog open onOpenChange={vi.fn()} title="T" description="D" disabled onConfirm={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Remove' })).toBeDisabled();
  });

  it('with softDisabled, Confirm stays focusable, is aria-disabled, refuses activation and is described by the reason', () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open
        onOpenChange={vi.fn()}
        title="T"
        description="D"
        disabled
        softDisabled
        confirmDescribedBy="why"
        onConfirm={onConfirm}
      >
        <p id="why">Needs updates.manage.</p>
      </ConfirmDialog>,
    );
    const button = screen.getByRole('button', { name: 'Remove' });
    expect(button).not.toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveAccessibleDescription('Needs updates.manage.');
    button.focus();
    expect(button).toHaveFocus();
    fireEvent.click(button);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('with softDisabled but not disabled, Confirm works and is not aria-disabled', () => {
    const onConfirm = vi.fn();
    render(<ConfirmDialog open onOpenChange={vi.fn()} title="T" description="D" softDisabled onConfirm={onConfirm} />);
    const button = screen.getByRole('button', { name: 'Remove' });
    expect(button).not.toHaveAttribute('aria-disabled', 'true');
    fireEvent.click(button);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('stays natively disabled while loading even with softDisabled, so a double click sends one request', () => {
    render(
      <ConfirmDialog open onOpenChange={vi.fn()} title="T" description="D" loading softDisabled onConfirm={vi.fn()} />,
    );
    expect(screen.getByRole('button', { name: 'Processing' })).toBeDisabled();
  });
});
