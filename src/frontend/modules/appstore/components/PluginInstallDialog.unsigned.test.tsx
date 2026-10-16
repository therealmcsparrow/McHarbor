// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Button } from '@resources/components/ui/Button';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { usePluginInstallFlow } from '../pages/usePluginInstallFlow';
import type { PluginEntry } from '../types/plugins';
import { PluginInstallFlowDialog } from './PluginInstallFlowDialog';

const baseEntry: PluginEntry = {
  slug: 'comm-thirdparty',
  name: 'Third Party',
  version: '2026.09.02',
  author: 'Someone <someone@example.com>',
  license: 'MIT',
  homepage: '',
  description: 'A plugin nobody signed',
  category: 'Communication',
  download_url: 'https://catalog.example.com/comm-thirdparty.tgz',
  versions: [{ version: '2026.09.02', download_url: 'https://catalog.example.com/comm-thirdparty.tgz' }],
};

const signedLooking: PluginEntry = {
  ...baseEntry,
  signature: 'c2lnbmF0dXJl',
  versions: [{ ...baseEntry.versions[0]!, signature: 'c2lnbmF0dXJl' }],
};

const REFUSED = {
  phase: 'error',
  step: 0,
  total: 100,
  status: 'error',
  error_code: 'err.plugin.unsigned_confirmation_required',
  error_text: 'plugin is unsigned and the install was not confirmed',
};
const DONE = {
  phase: 'done',
  step: 100,
  total: 100,
  status: 'done',
  result: { slug: 'comm-thirdparty', version: '2026.09.02', path: '', needs_enable: false },
};

function sse(...events: object[]) {
  return new HttpResponse(events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join(''), {
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

function Harness({ entry }: { entry: PluginEntry }) {
  const flow = usePluginInstallFlow(() => undefined);
  return (
    <>
      <Button onClick={() => flow.dispatchInstall(entry)}>open-install</Button>
      <PluginInstallFlowDialog flow={flow} />
    </>
  );
}

describe('PluginInstallDialog unsigned confirmation', () => {
  const calls = vi.fn();

  function answer(response: () => Response | Promise<Response>) {
    server.use(
      http.post('*/api/appstore/plugins/install/stream', async ({ request }) => {
        calls(await request.json());
        return response();
      }),
    );
  }

  async function open(entry: PluginEntry) {
    const user = userEvent.setup();
    renderWithProviders(<Harness entry={entry} />);
    await user.click(screen.getByRole('button', { name: 'open-install' }));
    return { user, dialog: await screen.findByRole('dialog') };
  }

  beforeEach(() => {
    calls.mockReset();
    Element.prototype.scrollIntoView = vi.fn();
    grantPermissions(['plugins.manage']);
    server.use(
      http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/auth/me/permissions', () =>
        HttpResponse.json({ success: true, data: { permissions: [], wildcard: true } }),
      ),
    );
    answer(() => sse(DONE));
  });

  it('sends nothing until the risk is acknowledged, then sends allow_unsigned', async () => {
    const { user, dialog } = await open(baseEntry);
    expect(within(dialog).getByText(/Third Party runs as native code inside the McHarbor process/)).toBeInTheDocument();
    const install = within(dialog).getByRole('button', { name: 'Install' });
    expect(install).toBeDisabled();
    await user.click(install);
    expect(calls).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('checkbox'));
    await user.click(within(dialog).getByRole('button', { name: 'Install' }));
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(1));
    expect(calls).toHaveBeenCalledWith(expect.objectContaining({ allow_unsigned: true }));
    expect(await within(dialog).findByRole('button', { name: 'Done' })).toBeInTheDocument();
  });

  it('asks for confirmation when the server refuses, and retries only after it is given', async () => {
    let refuse = true;
    answer(() => {
      const res = refuse ? sse(REFUSED) : sse(DONE);
      refuse = false;
      return res;
    });
    const { user, dialog } = await open(signedLooking);
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(1));
    expect(calls.mock.calls[0]?.[0]).not.toHaveProperty('allow_unsigned', true);

    const retry = await within(dialog).findByRole('button', { name: 'Install unsigned plugin' });
    expect(within(dialog).getByText(/could not verify this release against a trusted signing key/)).toBeInTheDocument();
    expect(retry).toBeDisabled();
    await user.click(retry);
    expect(calls).toHaveBeenCalledTimes(1);

    await user.click(within(dialog).getByRole('checkbox'));
    await user.click(within(dialog).getByRole('button', { name: 'Install unsigned plugin' }));
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(2));
    expect(calls.mock.calls[1]?.[0]).toMatchObject({ allow_unsigned: true });
    expect(await within(dialog).findByRole('button', { name: 'Done' })).toBeInTheDocument();
  });

  it('renders the confirm disabled, naming plugins.manage, when it is missing', async () => {
    grantPermissions([]);
    const { user, dialog } = await open(baseEntry);
    await user.click(within(dialog).getByRole('checkbox'));
    const install = within(dialog).getByRole('button', { name: 'Install' });
    expect(install).toBeDisabled();
    await user.click(install);
    expect(calls).not.toHaveBeenCalled();
    await user.hover(install.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the plugins.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message when the install fails', async () => {
    const message = 'running migrations: table plugin_comm_thirdparty_items already exists';
    answer(() =>
      sse({
        phase: 'error',
        step: 0,
        total: 100,
        status: 'error',
        error_code: 'err.plugin.install_failed',
        error_text: message,
      }),
    );
    const { user, dialog } = await open(baseEntry);
    await user.click(within(dialog).getByRole('checkbox'));
    await user.click(within(dialog).getByRole('button', { name: 'Install' }));
    expect(await within(dialog).findByText(message)).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('sends one request on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return sse(DONE);
    });
    const { user, dialog } = await open(baseEntry);
    await user.click(within(dialog).getByRole('checkbox'));
    const install = within(dialog).getByRole('button', { name: 'Install' });
    await user.dblClick(install);
    await user.click(install);
    release();
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Done' })).toBeInTheDocument());
    expect(calls).toHaveBeenCalledTimes(1);
  });
});
