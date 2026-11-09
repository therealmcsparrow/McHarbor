// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { BackupTempLocationField } from './BackupTempLocationField';

const NOT_WRITABLE = 'The temp folder is not writable by McHarbor.';
const NOT_WRITABLE_DETAIL = 'mkdir /srv/spool: read-only file system';

const status = (over: Record<string, unknown> = {}) => ({
  path: '/var/lib/mcharbor/backups/tmp',
  source: 'default',
  settingPath: '',
  envOverride: false,
  envInvalidCode: '',
  defaultPath: '/var/lib/mcharbor/backups/tmp',
  sizeBytes: 1024,
  entries: 2,
  activeRuns: 0,
  retiredPaths: [],
  ...over,
});

const about = (native: boolean, sandboxed: boolean) =>
  http.get('*/api/about', () =>
    HttpResponse.json({ success: true, data: { version: '2026.11.02', native, sandboxed } }),
  );

const getStatus = (over: Record<string, unknown> = {}) =>
  http.get('*/api/container-backups/temp', () => HttpResponse.json({ success: true, data: status(over) }));

const refuse = (code: string, error: string, detail?: string, spy?: (body: unknown) => void) =>
  http.put('*/api/container-backups/temp', async ({ request }) => {
    spy?.(await request.json());
    return HttpResponse.json({ success: false, error, code, detail }, { status: 400 });
  });

const field = () => screen.findByLabelText('Location') as Promise<HTMLInputElement>;
const saveButton = () => screen.getByRole('button', { name: 'Save' });
const pres = () => Array.from(document.querySelectorAll('pre')).map((el) => el.textContent);

async function typeAndSave(path: string) {
  const user = userEvent.setup();
  const input = await field();
  await user.clear(input);
  await user.type(input, path);
  await user.click(saveButton());
  return { user, input };
}

describe('BackupTempLocationField', () => {
  beforeEach(() => {
    grantPermissions(['backups.manage']);
    server.use(getStatus(), about(true, true));
  });

  it('shows the remedy for a sandboxed install', async () => {
    server.use(refuse('err.backup_temp.not_writable', NOT_WRITABLE, NOT_WRITABLE_DETAIL));
    renderWithProviders(<BackupTempLocationField />);
    await typeAndSave('/srv/spool');

    const alert = (await screen.findAllByRole('alert')).find((el) => el.textContent?.includes(NOT_WRITABLE));
    expect(alert).toBeDefined();
    expect(within(alert as HTMLElement).getByText(new RegExp(NOT_WRITABLE_DETAIL))).toBeInTheDocument();
    await waitFor(() =>
      expect(pres()).toEqual([
        "[Service]\nReadWritePaths='/srv/spool'",
        "sudo chown -R mcharbor:mcharbor '/srv/spool'",
      ]),
    );
    expect(screen.getByText(/backup-temp\.conf/)).toBeInTheDocument();
  });

  it('quotes a path holding a single quote in the remedy', async () => {
    server.use(refuse('err.backup_temp.not_writable', NOT_WRITABLE));
    renderWithProviders(<BackupTempLocationField />);
    await typeAndSave("/srv/it's");
    await waitFor(() => expect(pres()).toContain("sudo chown -R mcharbor:mcharbor '/srv/it'\\''s'"));
  });

  it('shows no commands for an unsandboxed native install', async () => {
    server.use(about(true, false), refuse('err.backup_temp.not_writable', NOT_WRITABLE));
    renderWithProviders(<BackupTempLocationField />);
    await typeAndSave('/srv/spool');
    await screen.findAllByText(NOT_WRITABLE);
    expect(pres()).toEqual([]);
  });

  it('shows the mount hint, not commands, for a container install', async () => {
    server.use(about(false, false), refuse('err.backup_temp.not_writable', NOT_WRITABLE));
    renderWithProviders(<BackupTempLocationField />);
    await typeAndSave('/srv/spool');
    expect(await screen.findByText(/Mount a host folder into the container at this path/)).toBeInTheDocument();
    expect(pres()).toEqual([]);
  });

  it('shows no remedy for a refusal other than not-writable', async () => {
    server.use(refuse('err.backup_temp.path_sensitive', 'That location is a protected system folder.'));
    renderWithProviders(<BackupTempLocationField />);
    await typeAndSave('/etc');
    await screen.findAllByText('That location is a protected system folder.');
    expect(pres()).toEqual([]);
    expect(screen.queryByText(/Mount a host folder/)).not.toBeInTheDocument();
  });

  it('clears the remedy when the typed value changes', async () => {
    server.use(refuse('err.backup_temp.not_writable', NOT_WRITABLE));
    renderWithProviders(<BackupTempLocationField />);
    const { user, input } = await typeAndSave('/srv/spool');
    await waitFor(() => expect(pres()).toHaveLength(2));
    await user.type(input, 'x');
    expect(pres()).toEqual([]);
  });

  it('keeps the typed value and the server message intact when a save is refused', async () => {
    const puts = vi.fn();
    server.use(refuse('err.backup_temp.not_writable', NOT_WRITABLE, NOT_WRITABLE_DETAIL, puts));
    renderWithProviders(<BackupTempLocationField />);
    const { input } = await typeAndSave('/srv/spool');
    await screen.findAllByText(NOT_WRITABLE);
    expect(puts).toHaveBeenCalledTimes(1);
    expect(puts).toHaveBeenCalledWith({ path: '/srv/spool' });
    expect(input).toHaveValue('/srv/spool');
    expect(input).toHaveAttribute('aria-invalid', 'true');
  });

  it('sends one request on a double click while the response is held open', async () => {
    const puts = vi.fn();
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.put('*/api/container-backups/temp', async ({ request }) => {
        puts(await request.json());
        await held;
        return HttpResponse.json({
          success: true,
          data: status({ source: 'setting', settingPath: '/srv/spool', path: '/srv/spool' }),
        });
      }),
    );
    renderWithProviders(<BackupTempLocationField />);
    const user = userEvent.setup();
    await user.type(await field(), '/srv/spool');
    const save = saveButton();
    await user.dblClick(save);
    await user.click(save);
    release();
    await waitFor(() => expect(puts).toHaveBeenCalled());
    expect(puts).toHaveBeenCalledTimes(1);
  });

  it('renders the field read-only with the override note when the environment variable is set', async () => {
    server.use(getStatus({ envOverride: true, source: 'env', path: '/data/spool', settingPath: '' }));
    renderWithProviders(<BackupTempLocationField />);
    const input = await field();
    await waitFor(() => expect(input).toHaveValue('/data/spool'));
    expect(input).toHaveAttribute('readonly');
    expect(input).not.toBeDisabled();
    expect(screen.getByText(/MCHARBOR_BACKUP_TEMP_DIR environment variable/)).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();
  });

  it('shows Save disabled and names backups.manage when the permission is missing', async () => {
    grantPermissions([]);
    renderWithProviders(<BackupTempLocationField />);
    const user = userEvent.setup();
    const save = saveButton();
    expect(save).toBeDisabled();
    await user.hover(save.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the backups.manage permission')).not.toHaveLength(0);
  });

  it('sends nothing when a denied user clicks Save', async () => {
    grantPermissions([]);
    const puts = vi.fn();
    server.use(refuse('err.backup_temp.not_writable', NOT_WRITABLE, undefined, puts));
    renderWithProviders(<BackupTempLocationField />);
    const user = userEvent.setup();
    await user.click(saveButton());
    expect(puts).not.toHaveBeenCalled();
  });

  it('is reachable by keyboard: a labelled input first, then Save once a change is typed', async () => {
    renderWithProviders(<BackupTempLocationField />);
    const user = userEvent.setup();
    const input = await field();
    expect(screen.getByRole('textbox', { name: 'Location' })).toBe(input);
    await user.tab();
    expect(input).toHaveFocus();
    await user.keyboard('/srv/spool');
    await user.tab();
    expect(saveButton()).toHaveFocus();
  });
});
