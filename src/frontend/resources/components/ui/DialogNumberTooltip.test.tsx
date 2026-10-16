// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './Dialog';
import { NumberInput } from './NumberInput';
import { SectionHeader } from './SectionHeader';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './Tooltip';

describe('dialog UI primitives', () => {
  it('wraps unstructured content in a dialog body', () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove container</DialogTitle>
            <DialogDescription>Confirm the action</DialogDescription>
          </DialogHeader>
          <p>Body text</p>
          <DialogFooter>Actions</DialogFooter>
        </DialogContent>
      </Dialog>,
    );

    expect(screen.getByRole('dialog')).toHaveClass('max-w-lg');
    expect(screen.getByText('Remove container')).toHaveClass('font-semibold');
    expect(screen.getByText('Body text').parentElement).toHaveClass('overflow-y-auto');
    expect(screen.getByText('Actions')).toHaveClass('border-t');
  });

  it('keeps explicit dialog body content unchanged', () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogBody className="custom-body">Explicit body</DialogBody>
        </DialogContent>
      </Dialog>,
    );

    expect(screen.getByText('Explicit body')).toHaveClass('custom-body');
  });
});

describe('NumberInput', () => {
  it('increments, decrements, parses input, and respects bounds', () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <NumberInput aria-label="Replicas" value={2} min={1} max={3} step={1} onChange={onChange} />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Increase' }));
    fireEvent.click(screen.getByRole('button', { name: 'Decrease' }));
    fireEvent.change(screen.getByLabelText('Replicas'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('Replicas'), { target: { value: '4' } });
    fireEvent.change(screen.getByLabelText('Replicas'), { target: { value: '-' } });
    fireEvent.change(screen.getByLabelText('Replicas'), { target: { value: 'abc' } });

    expect(onChange).toHaveBeenCalledWith(3);
    expect(onChange).toHaveBeenCalledWith(1);
    expect(onChange).toHaveBeenCalledTimes(3);

    rerender(<NumberInput aria-label="Replicas" value={1} min={1} max={3} onChange={onChange} size="sm" />);
    expect(screen.getByRole('button', { name: 'Decrease' })).toBeDisabled();
    expect(screen.getByLabelText('Replicas')).toHaveClass('mc-control', 'text-xs');

    rerender(<NumberInput aria-label="Replicas" value={3} min={1} max={3} onChange={onChange} disabled />);
    expect(screen.getByRole('button', { name: 'Increase' })).toBeDisabled();
  });
});

describe('SectionHeader', () => {
  it('renders optional description and action', () => {
    render(
      <SectionHeader
        title="Networking"
        description="Driver details"
        action={<span>Refresh</span>}
        className="section-extra"
      />,
    );

    expect(screen.getByText('Networking').closest('.section-extra')).toBeInTheDocument();
    expect(screen.getByText('Driver details')).toBeInTheDocument();
    expect(screen.getByText('Refresh')).toBeInTheDocument();
  });
});

describe('Tooltip', () => {
  it('renders tooltip content through the Radix provider', async () => {
    render(
      <TooltipProvider delayDuration={0}>
        <Tooltip open>
          <TooltipTrigger>Hover me</TooltipTrigger>
          <TooltipContent className="tip-extra">Helpful text</TooltipContent>
        </Tooltip>
      </TooltipProvider>,
    );

    expect((await screen.findAllByText('Helpful text'))[0]).toHaveClass('tip-extra');
  });
});
