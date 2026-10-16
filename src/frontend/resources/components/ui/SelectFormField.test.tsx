// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FormField } from './FormField';
import { Select, selectTriggerVariants } from './Select';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        'select.placeholder': 'Select an option',
        'select.searchPlaceholder': 'Search options',
        'select.noResults': 'No results',
      })[key] ?? key,
  }),
}));

describe('FormField', () => {
  it('renders label, children, and description', () => {
    render(
      <FormField label="Name" htmlFor="name" description="Visible in lists" className="field-extra">
        <input id="name" />
      </FormField>,
    );

    expect(screen.getByText('Name').closest('.field-extra')).toBeInTheDocument();
    expect(screen.getByLabelText('Name')).toBeInTheDocument();
    expect(screen.getByText('Visible in lists')).toHaveClass('text-xs');
  });
});

describe('Select', () => {
  it('opens, searches, selects an option, and resets search on close', async () => {
    const onChange = vi.fn();
    render(
      <Select
        ariaLabel="Environment"
        value=""
        onChange={onChange}
        options={[
          { value: 'local', label: 'Local Docker' },
          { value: 'prod', label: 'Production' },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Environment' }));
    fireEvent.change(await screen.findByPlaceholderText('Search options'), { target: { value: 'prod' } });

    expect(screen.queryByText('Local Docker')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Production' }));

    expect(onChange).toHaveBeenCalledWith('prod');
    await waitFor(() => expect(screen.queryByPlaceholderText('Search options')).not.toBeInTheDocument());
  });

  it('renders selected, disabled, non-searchable, and no-results states', async () => {
    const { rerender } = render(
      <Select
        ariaLabel="Environment"
        value="local"
        onChange={vi.fn()}
        options={[{ value: 'local', label: 'Local Docker' }]}
        disabled
        searchable={false}
        variant="outline"
        className="select-extra"
      />,
    );

    expect(screen.getByRole('button', { name: 'Environment' })).toHaveClass('select-extra');
    expect(screen.getByText('Local Docker')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Environment' }));
    expect(screen.queryByText('Search options')).not.toBeInTheDocument();

    rerender(
      <Select
        ariaLabel="Environment"
        value=""
        onChange={vi.fn()}
        options={[{ value: 'local', label: 'Local Docker' }]}
        searchable
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Environment' }));
    fireEvent.change(await screen.findByPlaceholderText('Search options'), { target: { value: 'missing' } });

    expect(screen.getByText('No results')).toBeInTheDocument();
    expect(selectTriggerVariants({ variant: 'outline' })).toContain('mc-field');
  });

  it('renders one header per consecutive group and keeps ungrouped rows headerless', async () => {
    render(
      <Select
        ariaLabel="Device"
        value=""
        onChange={vi.fn()}
        options={[
          { value: 'a', label: 'ttyUSB0', group: 'Serial' },
          { value: 'b', label: 'ttyACM0', group: 'Serial' },
          { value: 'c', label: 'card0', group: 'GPU' },
          { value: 'd', label: 'Enter manually' },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Device' }));
    expect(await screen.findAllByText('Serial')).toHaveLength(1);
    expect(screen.getAllByText('GPU')).toHaveLength(1);
    expect(screen.getAllByRole('presentation').map((node) => node.textContent)).toEqual(['Serial', 'GPU']);

    fireEvent.change(screen.getByPlaceholderText('Search options'), { target: { value: 'card' } });
    expect(screen.queryByText('Serial')).not.toBeInTheDocument();
    expect(screen.getByText('GPU')).toBeInTheDocument();
  });
});
