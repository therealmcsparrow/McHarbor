// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DeviceMappingEntry } from '../../types/edit-form';
import { DeviceMappingFields } from './DeviceMappingFields';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const devices = [
  {
    path: '/dev/sda',
    type: 'block',
    major: 8,
    minor: 0,
    category: 'block',
    label: 'Samsung SSD 870',
    stablePath: '/dev/disk/by-id/ata-Samsung_SSD_870',
    dangerous: true,
  },
  {
    path: '/dev/dri/renderD128',
    type: 'char',
    major: 226,
    minor: 128,
    category: 'gpu',
    label: 'i915',
    dangerous: false,
  },
  {
    path: '/dev/ttyUSB0',
    type: 'char',
    major: 188,
    minor: 0,
    category: 'serial',
    label: 'Silicon Labs CP2102',
    stablePath: '/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0',
    links: ['/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0'],
    dangerous: false,
  },
];

const EMPTY: DeviceMappingEntry = { pathOnHost: '', pathInContainer: '', cgroupPermissions: 'rwm' };

function Harness({
  initial = EMPTY,
  spy,
  taken,
}: {
  initial?: DeviceMappingEntry;
  spy: (next: DeviceMappingEntry) => void;
  taken?: string[];
}) {
  const [value, setValue] = useState(initial);
  return (
    <DeviceMappingFields
      idPrefix="row"
      value={value}
      onChange={(next) => {
        spy(next);
        setValue(next);
      }}
      onRemove={vi.fn()}
      takenContainerPaths={taken}
    />
  );
}

async function openHostPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Host Path' }));
}

describe('DeviceMappingFields', () => {
  const requests = vi.fn();

  beforeEach(() => {
    requests.mockReset();
    grantPermissions(['host.view', 'containers.manage']);
    server.use(
      http.get('*/api/system/host-devices', () => {
        requests();
        return HttpResponse.json({
          success: true,
          data: { devices, truncated: false, collectedAt: '2026-09-27T20:00:00Z' },
        });
      }),
    );
  });

  it('groups the host devices by kind in picker order, with manual entry last', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness spy={vi.fn()} />);
    await openHostPicker(user);
    const headers = (await screen.findAllByRole('presentation')).map((node) => node.textContent);
    expect(headers).toEqual(['Serial', 'GPU', 'Block']);
    const rows = screen.getAllByRole('button').map((node) => node.textContent ?? '');
    const serial = rows.findIndex((text) => text.includes('CP2102'));
    const block = rows.findIndex((text) => text.includes('Samsung'));
    const manual = rows.findIndex((text) => text.includes('Enter manually'));
    expect(serial).toBeGreaterThan(-1);
    expect(serial).toBeLessThan(block);
    expect(block).toBeLessThan(manual);
  });

  it('stores the stable by-id path and prefills the container path with the device node', async () => {
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(<Harness spy={spy} />);
    await openHostPicker(user);
    await user.click(screen.getByRole('button', { name: /Silicon Labs CP2102 \(\/dev\/ttyUSB0\)/ }));
    expect(spy).toHaveBeenLastCalledWith({
      pathOnHost: '/dev/serial/by-id/usb-Silicon_Labs_CP2102_0001-if00-port0',
      pathInContainer: '/dev/ttyUSB0',
      cgroupPermissions: 'rwm',
    });
    expect(screen.getByText('Resolves to /dev/ttyUSB0 on the host')).toBeInTheDocument();
    expect(screen.getByLabelText('Container Path')).toHaveValue('/dev/ttyUSB0');
  });

  it('keeps a container path the operator already typed', async () => {
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(<Harness spy={spy} initial={{ ...EMPTY, pathInContainer: '/dev/modem' }} />);
    await openHostPicker(user);
    await user.click(screen.getByRole('button', { name: /Silicon Labs CP2102/ }));
    expect(spy).toHaveBeenLastCalledWith(expect.objectContaining({ pathInContainer: '/dev/modem' }));
  });

  it('suggests the next free node when another device already uses the kernel node', async () => {
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(<Harness spy={spy} taken={['/dev/ttyUSB0']} />);
    await openHostPicker(user);
    await user.click(screen.getByRole('button', { name: /Silicon Labs CP2102/ }));
    expect(spy).toHaveBeenLastCalledWith(expect.objectContaining({ pathInContainer: '/dev/ttyUSB1' }));
    expect(screen.queryByText('Another device in this container already uses this path.')).not.toBeInTheDocument();
  });

  it('warns on a container path another device already uses and applies the free one', async () => {
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(
      <Harness
        spy={spy}
        taken={['/dev/ttyUSB0', '/dev/ttyUSB1']}
        initial={{ ...EMPTY, pathInContainer: '/dev/ttyUSB0' }}
      />,
    );
    expect(await screen.findByText('Another device in this container already uses this path.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Use /dev/ttyUSB2' }));
    expect(spy).toHaveBeenLastCalledWith(expect.objectContaining({ pathInContainer: '/dev/ttyUSB2' }));
    expect(screen.queryByText('Another device in this container already uses this path.')).not.toBeInTheDocument();
  });

  it('marks a block device with a warning badge', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness spy={vi.fn()} />);
    await openHostPicker(user);
    await user.click(screen.getByRole('button', { name: /Samsung SSD 870/ }));
    expect(screen.getByText('Block device')).toBeInTheDocument();
    expect(screen.getByText(/can destroy the host's data/)).toBeInTheDocument();
  });

  it('escapes to manual entry for a device that is not listed', async () => {
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(<Harness spy={spy} />);
    await openHostPicker(user);
    await user.click(screen.getByRole('button', { name: /Enter manually/ }));
    const input = screen.getByLabelText('Host Path');
    await user.type(input, '/dev/ttyUSB9');
    expect(input).toHaveValue('/dev/ttyUSB9');
    expect(spy).toHaveBeenLastCalledWith({
      pathOnHost: '/dev/ttyUSB9',
      pathInContainer: '/dev/ttyUSB9',
      cgroupPermissions: 'rwm',
    });
    expect(screen.getByRole('button', { name: 'Choose from list' })).toBeEnabled();
  });

  it('opens in manual mode for a saved path the host no longer lists', async () => {
    renderWithProviders(
      <Harness spy={vi.fn()} initial={{ ...EMPTY, pathOnHost: '/dev/ttyUSB7', pathInContainer: '/dev/ttyUSB7' }} />,
    );
    await waitFor(() => expect(requests).toHaveBeenCalled());
    expect(await screen.findByLabelText('Host Path')).toHaveValue('/dev/ttyUSB7');
  });

  it('offers the cgroup permission presets with rwm as the default', async () => {
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(<Harness spy={spy} />);
    const trigger = screen.getByRole('button', { name: 'Permissions' });
    expect(trigger).toHaveTextContent('rwm');
    await user.click(trigger);
    expect(screen.getByText('Read and write')).toBeInTheDocument();
    expect(screen.getByText('Create the node only (mknod)')).toBeInTheDocument();
    const readOnly = screen.getByText('Read only').closest('button');
    expect(readOnly).not.toBeNull();
    await user.click(readOnly as HTMLElement);
    expect(spy).toHaveBeenLastCalledWith(expect.objectContaining({ cgroupPermissions: 'r' }));
  });

  it('without host.view it never asks the host and shows the list as denied', async () => {
    grantPermissions(['containers.manage']);
    const user = userEvent.setup();
    const spy = vi.fn();
    renderWithProviders(<Harness spy={spy} />);
    const input = await screen.findByLabelText('Host Path');
    const choose = screen.getByRole('button', { name: 'Choose from list' });
    expect(choose).toBeDisabled();
    await user.hover(choose.parentElement as HTMLElement);
    expect(await within(document.body).findAllByText(/host\.view/)).not.toHaveLength(0);
    await user.type(input, '/dev/kvm');
    expect(spy).toHaveBeenLastCalledWith(expect.objectContaining({ pathOnHost: '/dev/kvm' }));
    expect(requests).not.toHaveBeenCalled();
  });
});
