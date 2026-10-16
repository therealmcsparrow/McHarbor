// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ContainerInspect } from '@core/types/docker';
import { renderWithProviders } from '../../../test/render';
import type { useContainerConfig } from '../hooks/useContainerConfig';
import { containerToEditForm } from '../types/edit-form-parse';
import { ContainerConfigEditTabs } from './ContainerConfigEditTabs';

const container = {
  Id: 'c1',
  Name: '/homeassistant',
  Mounts: [{ Type: 'bind', Source: '/srv/ha', Destination: '/config', Mode: '', RW: true, Propagation: 'rprivate' }],
} as ContainerInspect;

describe('configuration editor', () => {
  it('offers the mount editor, with its warning, in its own Mounts section', () => {
    const config: ReturnType<typeof useContainerConfig> = {
      editing: true,
      data: containerToEditForm(container),
      dirty: false,
      isSaving: false,
      changes: {
        hasResourceChanges: false,
        hasConfigChanges: false,
        changedResourceFields: [],
        changedConfigFields: [],
      },
      start: vi.fn(),
      cancel: vi.fn(),
      save: vi.fn(),
      setField: vi.fn(),
    };
    renderWithProviders(
      <ContainerConfigEditTabs tab="mounts" onTabChange={() => {}} container={container} config={config} />,
    );
    expect(screen.getByRole('tab', { name: 'Mounts' })).toBeInTheDocument();
    expect(screen.getByText('Changing mounts recreates the container')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Destination of mount 1' })).toHaveValue('/config');
  });
});
