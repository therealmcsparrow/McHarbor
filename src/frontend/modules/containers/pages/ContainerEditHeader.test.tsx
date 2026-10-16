// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import type { ContainerInspect } from '@core/types/docker';
import { ContainerEditHeader } from './ContainerEditHeader';

const container = {
  Id: 'abcdef0123456789',
  Name: '/mcharbor',
  Image: 'sha256:1',
  Config: { Image: 'mcharbor-local:debug' },
  State: { Status: 'running' },
} as unknown as ContainerInspect;

function props(overrides: Partial<Parameters<typeof ContainerEditHeader>[0]> = {}) {
  return {
    container,
    isProtected: true,
    canUnlock: true,
    editLocked: true,
    editUnlocked: false,
    onBack: vi.fn(),
    onToggleLock: vi.fn(),
    editing: false,
    dirty: false,
    saving: false,
    onSave: vi.fn(),
    onCancel: vi.fn(),
    ...overrides,
  };
}

describe('container edit head', () => {
  let band: HTMLDivElement;

  beforeEach(() => {
    band = document.createElement('div');
    band.id = 'pagehead-slot';
    document.body.appendChild(band);
  });

  afterEach(() => {
    band.remove();
    resetPermissions();
  });

  it('puts the identity, the state and the actions in the page-head band', async () => {
    renderWithProviders(<ContainerEditHeader {...props()} />);
    await waitFor(() => expect(within(band).getByRole('heading', { name: 'mcharbor' })).toBeInTheDocument());
    expect(within(band).getByText('Configuration')).toBeInTheDocument();
    expect(within(band).getByText('Locked')).toBeInTheDocument();
    expect(within(band).getByText('mcharbor-local:debug')).toBeInTheDocument();
    expect(within(band).getByRole('button', { name: 'Back to container' })).toBeInTheDocument();
    expect(within(band).getByRole('button', { name: 'Unlock protected editing' })).toBeInTheDocument();
    expect(band.children).toHaveLength(1);
  });

  it('wires back and the lock toggle to their handlers', async () => {
    const user = userEvent.setup();
    const p = props();
    renderWithProviders(<ContainerEditHeader {...p} />);
    await user.click(await within(band).findByRole('button', { name: 'Back to container' }));
    await user.click(within(band).getByRole('button', { name: 'Unlock protected editing' }));
    expect(p.onBack).toHaveBeenCalledTimes(1);
    expect(p.onToggleLock).toHaveBeenCalledTimes(1);
  });

  it('offers no unlock for a container that cannot be unlocked', async () => {
    renderWithProviders(<ContainerEditHeader {...props({ canUnlock: false })} />);
    await within(band).findByRole('button', { name: 'Back to container' });
    expect(screen.queryByRole('button', { name: /Unlock protected editing|Lock editing/ })).not.toBeInTheDocument();
  });

  it('replaces Back with Save and Cancel while editing', async () => {
    grantPermissions(['containers.manage']);
    renderWithProviders(<ContainerEditHeader {...props({ editing: true, editLocked: false })} />);
    expect(await within(band).findByRole('button', { name: 'Save Changes' })).toBeInTheDocument();
    expect(within(band).getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
    expect(within(band).queryByRole('button', { name: 'Back to container' })).not.toBeInTheDocument();
  });

  it('keeps Save disabled until something changed, then saves; Cancel always reaches its handler', async () => {
    grantPermissions(['containers.manage']);
    const user = userEvent.setup();
    const clean = props({ editing: true, editLocked: false });
    const { unmount } = renderWithProviders(<ContainerEditHeader {...clean} />);
    expect(await within(band).findByRole('button', { name: 'Save Changes' })).toBeDisabled();
    await user.click(within(band).getByRole('button', { name: 'Cancel' }));
    expect(clean.onCancel).toHaveBeenCalledTimes(1);
    unmount();

    const dirty = props({ editing: true, editLocked: false, dirty: true });
    renderWithProviders(<ContainerEditHeader {...dirty} />);
    await user.click(await within(band).findByRole('button', { name: 'Save Changes' }));
    expect(dirty.onSave).toHaveBeenCalledTimes(1);
  });

  it('renders Save disabled for a user without containers.manage', async () => {
    grantPermissions([]);
    const p = props({ editing: true, editLocked: false, dirty: true });
    renderWithProviders(<ContainerEditHeader {...p} />);
    const save = await within(band).findByRole('button', { name: /Save Changes/ });
    expect(save).toBeDisabled();
  });
});
