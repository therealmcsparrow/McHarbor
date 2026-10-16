// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { NumberInput } from './NumberInput';

function NullableHarness({ onChange }: { onChange: (value: number | null) => void }) {
  const [value, setValue] = useState<number | null>(4);
  return (
    <NumberInput
      nullable
      aria-label="Quota"
      value={value}
      min={2}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
    />
  );
}

describe('NumberInput extensions', () => {
  it('lets a nullable field be cleared, and steps from min when empty', () => {
    const onChange = vi.fn();
    render(<NullableHarness onChange={onChange} />);

    fireEvent.change(screen.getByLabelText('Quota'), { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith(null);
    expect(screen.getByLabelText('Quota')).toHaveValue(null);

    fireEvent.click(screen.getByRole('button', { name: 'Increase' }));
    expect(onChange).toHaveBeenLastCalledWith(2);
    expect(screen.getByLabelText('Quota')).toHaveValue(2);
  });

  it('never emits an empty value for a required field and restores it on blur', () => {
    const onChange = vi.fn();
    render(<NumberInput aria-label="Port" value={22} min={1} max={65535} onChange={onChange} />);
    const input = screen.getByLabelText('Port');

    fireEvent.change(input, { target: { value: '' } });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.blur(input);
    expect(input).toHaveValue(22);
  });

  it('keeps an in-progress draft below min, emits once in range, and clamps on blur', () => {
    const onChange = vi.fn();
    render(<NumberInput aria-label="Minutes" value={30} min={5} max={240} onChange={onChange} />);
    const input = screen.getByLabelText('Minutes');

    fireEvent.change(input, { target: { value: '1' } });
    expect(input).toHaveValue(1);
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: '12' } });
    expect(onChange).toHaveBeenCalledWith(12);

    fireEvent.change(input, { target: { value: '1' } });
    fireEvent.blur(input);
    expect(onChange).toHaveBeenLastCalledWith(5);
    expect(input).toHaveValue(5);

    fireEvent.change(input, { target: { value: '999' } });
    fireEvent.blur(input);
    expect(onChange).toHaveBeenLastCalledWith(240);
  });

  it('rounds decimal steps to the step precision and clamps to the bounds', () => {
    const onChange = vi.fn();
    const { rerender } = render(<NumberInput aria-label="CPUs" value={0.2} step={0.1} min={0} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: 'Increase' }));
    expect(onChange).toHaveBeenLastCalledWith(0.3);

    rerender(<NumberInput aria-label="CPUs" value={7} step={5} min={5} max={10} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: 'Decrease' }));
    expect(onChange).toHaveBeenLastCalledWith(5);
    fireEvent.click(screen.getByRole('button', { name: 'Increase' }));
    expect(onChange).toHaveBeenLastCalledWith(10);
  });

  it('emits typed values outside the bounds when a form validates them itself', () => {
    const onChange = vi.fn();
    render(<NumberInput aria-label="Environments" value={4} min={1} max={48} allowOutOfRange onChange={onChange} />);

    fireEvent.change(screen.getByLabelText('Environments'), { target: { value: '49' } });
    expect(onChange).toHaveBeenCalledWith(49);
  });

  it('renders a unit suffix and locks the steppers when read-only', () => {
    render(<NumberInput aria-label="Memory" value={512} suffix="MiB" readOnly onChange={() => undefined} />);

    expect(screen.getByText('MiB')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Increase' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Decrease' })).toBeDisabled();
  });
});
