// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, renderHook, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../../test/render';
import { useNetworkEditHandlers } from '../../hooks/useNetworkTabHooks';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { containerToEditForm } from '../../types/edit-form-parse';
import { NetworkTabPortsSection } from './NetworkTabPortsSection';

describe('exposed ports while editing', () => {
  it('offers removal only for ports neither the image nor a mapping holds', () => {
    const onRemove = vi.fn();
    renderWithProviders(
      <NetworkTabPortsSection
        containerId="c1"
        allowConnectionTest={false}
        editing
        exposedPorts={[]}
        exposedRows={[{ port: '8123/tcp', lockedBy: 'image' }, { port: '9000/tcp' }]}
        onRemoveExposedPort={onRemove}
        portMappingKeys={[]}
        portMappings={[]}
        publishedPortMappings={[]}
        onAddPortMapping={() => {}}
        onPortMappingChange={() => {}}
        onRemovePortMapping={() => {}}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Remove exposed port 9000/tcp' }));
    expect(onRemove).toHaveBeenCalledWith('9000/tcp');
    expect(screen.queryByRole('button', { name: 'Remove exposed port 8123/tcp' })).not.toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Declared by the image' })).toBeInTheDocument();
  });

  it('removing a port mapping also stops exposing its container port', () => {
    const onFieldChange = vi.fn();
    const editData: EditFormData = {
      ...containerToEditForm({ Id: 'c1', Name: '/c1' } as ContainerInspect),
      portMappings: [{ containerPort: '8080', hostPort: '18080', hostIp: '0.0.0.0', protocol: 'tcp' }],
      exposedPorts: ['8080/tcp', '8123/tcp'],
    };
    const { result } = renderHook(() => useNetworkEditHandlers(editData, [], onFieldChange, new Set(['8123/tcp'])));

    result.current.handleRemovePortMapping(0);
    expect(onFieldChange).toHaveBeenCalledWith('portMappings', []);
    expect(onFieldChange).toHaveBeenCalledWith('exposedPorts', ['8123/tcp']);
  });
});
