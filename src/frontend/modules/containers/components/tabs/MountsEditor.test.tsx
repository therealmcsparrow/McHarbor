// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../../test/render';
import type { MountEntry } from '../../types/edit-form';
import { MountsEditor } from './MountsEditor';

const config: MountEntry = { type: 'bind', source: '/srv/ha', target: '/config', readOnly: false, propagation: '' };

describe('MountsEditor', () => {
  it('warns that changing mounts recreates the container and moves no data', () => {
    renderWithProviders(<MountsEditor mounts={[config]} onChange={() => {}} />);
    expect(screen.getByText('Changing mounts recreates the container')).toBeInTheDocument();
    expect(screen.getByText(/Data is not moved/)).toBeInTheDocument();
  });

  it('adds and removes mounts', () => {
    const onChange = vi.fn();
    renderWithProviders(<MountsEditor mounts={[config]} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: 'Add mount' }));
    expect(onChange).toHaveBeenLastCalledWith([
      config,
      { type: 'bind', source: '', target: '', readOnly: false, propagation: '' },
    ]);

    fireEvent.click(screen.getByRole('button', { name: 'Remove mount /config' }));
    expect(onChange).toHaveBeenLastCalledWith([]);
  });

  it('edits a target in place', () => {
    const onChange = vi.fn();
    renderWithProviders(<MountsEditor mounts={[config]} onChange={onChange} />);
    fireEvent.change(screen.getByRole('textbox', { name: 'Destination of mount 1' }), { target: { value: '/data' } });
    expect(onChange).toHaveBeenLastCalledWith([{ ...config, target: '/data' }]);
  });
});
