// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { VMListTabs } from '../../../plugins/Plugins/addon-kvm/frontend/src/pages/VMListTabs';
import { DisplayProfilesTable } from '../../../plugins/Plugins/addon-kvm/frontend/src/pages/DisplayProfilesTable';
import type { DisplayProfile } from '../../../plugins/Plugins/addon-kvm/frontend/src/types-display';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

function profile(overrides: Partial<DisplayProfile>): DisplayProfile {
  return {
    id: 'p1',
    name: 'office',
    builtin: false,
    imageCompression: 'auto_lz',
    jpegWanCompression: 'auto',
    zlibGlzWanCompression: 'never',
    streamingVideo: 'filter',
    playbackCompression: 'on',
    agentMouse: 'on',
    vmCount: 0,
    createdAt: '2026-10-01T00:00:00Z',
    updatedAt: '2026-10-01T00:00:00Z',
    ...overrides,
  };
}

const fastLan = profile({
  id: 'builtin-fast-lan',
  name: 'Fast LAN',
  builtin: true,
  imageCompression: 'off',
  vmCount: 4,
});
const office = profile({ id: 'custom-1', name: 'office', vmCount: 3 });

describe('addon-kvm display profiles', () => {
  const deletes = vi.fn();
  const creates = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  function answerDelete(respond: () => Response | Promise<Response>) {
    server.use(
      http.delete('*/api/vms/display-profiles/:id', ({ request, params }) => {
        deletes(params.id, new URL(request.url).searchParams.get('reassign'));
        return respond();
      }),
    );
  }

  // The page header portals its actions into the shell's band; the test is
  // the shell here, so it provides the band.
  afterEach(() => {
    document.getElementById('pagehead-slot')?.remove();
  });

  beforeEach(() => {
    const band = document.createElement('div');
    band.id = 'pagehead-slot';
    document.body.appendChild(band);
    deletes.mockReset();
    creates.mockReset();
    grantPermissions(['vm.view', 'vm.manage']);
    server.use(
      http.get('*/api/vms/display-profiles', () => HttpResponse.json({ success: true, data: [fastLan, office] })),
      http.post('*/api/vms/display-profiles', async ({ request }) => {
        creates(await request.json());
        return HttpResponse.json({ success: true, data: profile({ id: 'new' }) }, { status: 201 });
      }),
    );
    answerDelete(() => HttpResponse.json({ success: true, code: 'msg.vm.display_profile_deleted' }));
  });

  async function rowFor(name: string) {
    const cell = await screen.findByText(name, { selector: 'span' });
    return cell.closest('tr') as HTMLElement;
  }

  async function openDelete(user: ReturnType<typeof userEvent.setup>) {
    const row = await rowFor('office');
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    return screen.findByRole('dialog');
  }

  it('places the Display profiles tab between Virtual machines and Conversion', () => {
    renderWithProviders(<VMListTabs tab="machines" onChange={() => undefined} />);
    const labels = screen.getAllByRole('button').map((button) => button.textContent);
    expect(labels).toEqual(['Virtual machines', 'Display profiles', 'Conversion']);
  });

  it('offers only duplicate on a built-in, and names why edit and delete are off', async () => {
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    const row = await rowFor('Fast LAN');
    expect(within(row).getByText('Built-in')).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: 'Edit' })).toBeDisabled();
    expect(within(row).getByRole('button', { name: 'Delete' })).toBeDisabled();
    expect(within(row).getByRole('button', { name: 'Duplicate' })).toBeEnabled();
    await user.hover(within(row).getByRole('button', { name: 'Delete' }).parentElement as HTMLElement);
    expect(await screen.findAllByText('Built-in profiles are read-only. Duplicate one to change it.')).not.toHaveLength(
      0,
    );

    const custom = await rowFor('office');
    expect(within(custom).getByRole('button', { name: 'Edit' })).toBeEnabled();
    expect(within(custom).getByRole('button', { name: 'Delete' })).toBeEnabled();
  });

  it('deletes nothing until the operator confirms, and says the guests move to Fast LAN', async () => {
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    const dialog = await openDelete(user);
    expect(dialog).toHaveTextContent('3 virtual machines use this profile.');
    expect(deletes).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(deletes).not.toHaveBeenCalled();

    const again = await openDelete(user);
    await user.click(within(again).getByRole('button', { name: 'Move to Fast LAN and delete' }));
    await waitFor(() => expect(deletes).toHaveBeenCalledWith('custom-1', 'true'));
  });

  it('renders delete disabled, naming vm.manage, when it is not held', async () => {
    grantPermissions(['vm.view']);
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    const row = await rowFor('office');
    const button = within(row).getByRole('button', { name: 'Delete' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(deletes).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the vm.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open with the server message intact when the delete is refused', async () => {
    const detail = 'In use by 5 virtual machine(s).';
    answerDelete(() =>
      HttpResponse.json(
        { success: false, code: 'err.vm.display_profile_in_use', error: detail, data: { inUse: 5 } },
        { status: 409 },
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    const dialog = await openDelete(user);
    await user.click(within(dialog).getByRole('button', { name: 'Move to Fast LAN and delete' }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert).toHaveTextContent('Virtual machines still use this display profile.');
    expect(alert).toHaveTextContent(detail);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('sends one delete on a double click while the response is held open', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answerDelete(async () => {
      await held;
      return HttpResponse.json({ success: true });
    });
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    const dialog = await openDelete(user);
    const confirm = within(dialog).getByRole('button', { name: 'Move to Fast LAN and delete' });
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(deletes).toHaveBeenCalledTimes(1);
  });

  it('will not create a profile without a name, then sends the Fast LAN defaults', async () => {
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    await user.click(await screen.findByRole('button', { name: 'New profile' }));
    const dialog = await screen.findByRole('dialog');
    const save = within(dialog).getByRole('button', { name: 'Save profile' });
    expect(save).toBeDisabled();
    expect(dialog).toHaveTextContent('A profile needs a name.');
    await user.click(save);
    expect(creates).not.toHaveBeenCalled();

    await user.type(within(dialog).getByLabelText('Name'), '  windows-lan  ');
    expect(save).toBeEnabled();
    await user.click(save);
    await waitFor(() => expect(creates).toHaveBeenCalledTimes(1));
    expect(creates.mock.calls[0]?.[0]).toEqual({
      name: 'windows-lan',
      description: '',
      imageCompression: 'off',
      jpegWanCompression: 'never',
      zlibGlzWanCompression: 'never',
      streamingVideo: 'filter',
      playbackCompression: 'on',
      agentMouse: 'on',
    });
  });

  it('keeps the create dialog open on a name the server refuses', async () => {
    server.use(
      http.post('*/api/vms/display-profiles', () =>
        HttpResponse.json({ success: false, code: 'err.vm.display_profile_name_taken' }, { status: 409 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<DisplayProfilesTable />);
    await user.click(await screen.findByRole('button', { name: 'New profile' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Name'), 'office');
    await user.click(within(dialog).getByRole('button', { name: 'Save profile' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      'A display profile with that name already exists.',
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
