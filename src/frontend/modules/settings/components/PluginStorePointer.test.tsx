// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import { PluginStorePointer } from './PluginStorePointer';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';

describe('PluginStorePointer', () => {
  it('says where plugin prereleases are set and links to the Plugin Store tab', () => {
    grantPermissions(['plugins.view']);
    renderWithProviders(
      <MemoryRouter>
        <PluginStorePointer />
      </MemoryRouter>,
    );
    expect(screen.getByText(/Plugin prereleases are set in the Plugin Store/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Plugin Store' })).toHaveAttribute('href', '/store?tab=plugins');
  });

  it('keeps the sentence but drops the link for a reader without plugins.view', () => {
    grantPermissions([]);
    renderWithProviders(
      <MemoryRouter>
        <PluginStorePointer />
      </MemoryRouter>,
    );
    expect(screen.getByText(/Plugin prereleases are set in the Plugin Store/)).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Open Plugin Store' })).not.toBeInTheDocument();
  });
});
