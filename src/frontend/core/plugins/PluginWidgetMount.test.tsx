// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { renderWithProviders } from '../../test/render';
import { createPluginWidgetComponent } from './PluginWidgetMount';

describe('plugin widget mount failure', () => {
  it('names the plugin whose widget threw, and keeps its message', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const Widget = createPluginWidgetComponent('sample-widget', 'overview', {
      mount: () => {
        throw new Error('chart library missing');
      },
    });
    renderWithProviders(
      <MemoryRouter>
        <Widget colSpan={4} typeId="containers" />
      </MemoryRouter>,
    );
    expect(screen.getByText(/The sample-widget add-on could not display this\./)).toBeInTheDocument();
    expect(screen.getByText(/chart library missing/)).toBeInTheDocument();
  });
});
