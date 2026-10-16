// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Checkbox } from './Checkbox';
import { InfoRow } from './InfoRow';
import { Input, inputVariants } from './Input';
import { Label } from './Label';
import { ListItem } from './ListItem';
import { Spinner } from './Spinner';
import { Switch } from './Switch';
import { Textarea, textareaVariants } from './Textarea';

describe('form UI primitives', () => {
  it('renders Input and Textarea variants', () => {
    render(
      <>
        <Input aria-label="Name" variant="outline" className="input-extra" defaultValue="McHarbor" />
        <Textarea aria-label="Description" variant="outline" className="textarea-extra" defaultValue="Desc" />
      </>,
    );

    expect(screen.getByLabelText('Name')).toHaveClass('mc-field', 'px-3', 'input-extra');
    expect(screen.getByLabelText('Description')).toHaveClass('min-h-[80px]', 'textarea-extra');
    expect(inputVariants({ variant: 'default' })).toContain('mc-field');
    expect(textareaVariants({ variant: 'default' })).toContain('mc-field');
  });

  it('renders Label and InfoRow', () => {
    render(
      <>
        <Label htmlFor="field" className="label-extra">
          Field
        </Label>
        <InfoRow label="Status" className="row-extra">
          Running
        </InfoRow>
      </>,
    );

    expect(screen.getByText('Field')).toHaveClass('text-foreground', 'label-extra');
    expect(screen.getByText('Status').parentElement).toHaveClass('row-extra');
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders ListItem optional sections', () => {
    render(
      <ListItem
        title="Container"
        subtitle="nginx"
        badge={<span>running</span>}
        actions={<button type="button">Open</button>}
        className="item-extra"
      />,
    );

    expect(screen.getByText('Container')).toBeInTheDocument();
    expect(screen.getByText('nginx')).toBeInTheDocument();
    expect(screen.getByText('running')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument();
    expect(screen.getByText('Container').closest('.item-extra')).toBeInTheDocument();
  });

  it('renders Spinner sizes', () => {
    const { container } = render(<Spinner size="xl" className="spin-extra" />);

    expect(container.firstChild).toHaveClass('h-8', 'spin-extra');
  });

  it('renders Checkbox checked and indeterminate indicators', () => {
    const onCheckedChange = vi.fn();
    const { rerender } = render(<Checkbox checked onCheckedChange={onCheckedChange} aria-label="Select" />);

    fireEvent.click(screen.getByRole('checkbox', { name: 'Select' }));

    expect(onCheckedChange).toHaveBeenCalled();
    expect(screen.getByRole('checkbox', { name: 'Select' })).toHaveAttribute('data-state', 'checked');

    rerender(<Checkbox checked="indeterminate" aria-label="Mixed" />);
    expect(screen.getByRole('checkbox', { name: 'Mixed' })).toHaveAttribute('data-state', 'indeterminate');
  });

  it('renders Switch and emits changes', () => {
    const onCheckedChange = vi.fn();
    render(<Switch checked={false} onCheckedChange={onCheckedChange} aria-label="Enabled" />);

    fireEvent.click(screen.getByRole('switch', { name: 'Enabled' }));

    expect(onCheckedChange).toHaveBeenCalledWith(true);
    expect(screen.getByRole('switch', { name: 'Enabled' })).toHaveClass('peer');
  });
});
