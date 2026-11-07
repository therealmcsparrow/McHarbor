// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { focusManager } from '@tanstack/react-query';
import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginReturnDialog } from './PluginReturnDialog';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

type Check = Record<string, unknown>;

const checks = vi.fn();
const returns = vi.fn();
const overrides = vi.fn();
let checkBody: Check = {};
let checkStatus = 200;

const CLEAR: Check = { ok: true, outcome: 'clear', installed_version: '2026.11.02-rc.1', target_version: '2026.11.01' };

function setup(check: Check = CLEAR) {
  checkBody = check;
  checkStatus = 200;
}

function mount() {
  const onOpenChange = vi.fn();
  const { client } = renderWithProviders(
    <PluginReturnDialog slug="alpha" name="Alpha" installedVersion="2026.11.02-rc.1" onOpenChange={onOpenChange} />,
  );
  return { onOpenChange, client };
}

async function confirm() {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).getByRole('button', { name: 'Return to stable' });
}

function expectBlocked(button: HTMLElement) {
  expect(button).toHaveAttribute('aria-disabled', 'true');
}

async function ready() {
  const button = await confirm();
  await waitFor(() => expect(button).not.toHaveAttribute('aria-disabled', 'true'));
  return button;
}

beforeEach(() => {
  checks.mockReset();
  returns.mockReset();
  overrides.mockReset();
  setup();
  grantPermissions(['plugins.manage']);
  server.use(
    http.get('*/api/appstore/plugins/:slug/return/check', () => {
      checks();
      return HttpResponse.json(
        checkStatus === 200
          ? { success: true, data: checkBody }
          : { success: false, error: 'The catalog could not be read: connection reset', code: 'err.x' },
        { status: checkStatus },
      );
    }),
    http.post('*/api/appstore/plugins/:slug/return', async ({ request }) => {
      const text = await request.text();
      returns(text ? JSON.parse(text) : undefined);
      return HttpResponse.json({ success: true, data: { slug: 'alpha', from: '2026.11.02-rc.1', to: '2026.11.01' } });
    }),
    http.put('*/api/appstore/plugins/:slug/channel', async ({ request }) => {
      overrides(await request.json());
      return HttpResponse.json({ success: true, data: { slug: 'alpha', prereleases: null } });
    }),
    http.get('*/api/appstore/plugins/*', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
  );
});

describe('PluginReturnDialog destructive control', () => {
  it('sends nothing when cancelled, and checks once', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await ready();
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(returns).not.toHaveBeenCalled();
    expect(checks).toHaveBeenCalledTimes(1);
  });

  it('moves only after confirmation, with no consent flag, leaves the override to the server and closes', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    expect(returns).not.toHaveBeenCalled();
    await user.click(await ready());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(returns).toHaveBeenCalledTimes(1);
    expect(returns).toHaveBeenCalledWith({});
    expect(overrides).not.toHaveBeenCalled();
  });

  it('is disabled naming plugins.manage and runs no check when the key is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    const button = await confirm();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(returns).not.toHaveBeenCalled();
    expect(checks).not.toHaveBeenCalled();
    expect(await screen.findByText('Needs plugins.manage.')).toBeInTheDocument();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the plugins.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message when the return is refused', async () => {
    const message = 'Cannot return: state changed after the check, migration 004_extra.sql is applied';
    server.use(
      http.post('*/api/appstore/plugins/:slug/return', () =>
        HttpResponse.json(
          { success: false, error: message, code: 'err.update.migration_blocks_stable' },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await user.click(await ready());
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expectBlocked(await confirm());
    expect(screen.getByRole('button', { name: 'Check again' })).toBeEnabled();
  });

  it('sends one request on a double click, with the response held open', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/appstore/plugins/:slug/return', async ({ request }) => {
        returns(await request.json());
        await held;
        return HttpResponse.json({ success: true, data: { slug: 'alpha', from: 'a', to: 'b' } });
      }),
    );
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    const button = await ready();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(returns).toHaveBeenCalledTimes(1);
  });
});

describe('PluginReturnDialog check outcomes', () => {
  it('names the blocking migration and keeps Confirm disabled, with Close only', async () => {
    setup({ ok: false, outcome: 'blocked', blocking_migration: '003_prerelease_table.sql' });
    mount();
    const region = await screen.findByRole('alert');
    await waitFor(() => expect(region).toHaveTextContent('003_prerelease_table.sql'));
    expectBlocked(await confirm());
    expect(screen.queryByRole('button', { name: 'Cancel' })).not.toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Close' }).length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: 'Check again' })).not.toBeInTheDocument();
  });

  it('says there is no stable version and keeps Confirm disabled', async () => {
    setup({ ok: false, outcome: 'no-stable', installed_version: '2026.11.02-rc.1' });
    mount();
    expect(await screen.findByText('No stable version of Alpha is available.')).toBeInTheDocument();
    expectBlocked(await confirm());
  });

  it('shows a prerelease host as disabled with the reason', async () => {
    setup({ ok: false, outcome: 'prerelease-host' });
    mount();
    expect(await screen.findByText(/Plugins follow this prerelease host/)).toBeInTheDocument();
    expectBlocked(await confirm());
  });

  it('treats an outcome it does not know as blocked', async () => {
    setup({ ok: true, outcome: 'from-the-future', blocking_migration: 'ignored.sql' });
    mount();
    expect(await screen.findByText(/Cannot return: the check gave a result this version does not know/)).toBeVisible();
    expectBlocked(await confirm());
  });

  it('never checks again on its own when the window regains focus', async () => {
    const { client } = mount();
    await ready();
    focusManager.setFocused(false);
    focusManager.setFocused(true);
    // QueryClient's focus listener awaits resumePausedMutations before it refetches; drain those microtasks.
    await act(async () => {});
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(checks).toHaveBeenCalledTimes(1);
  });

  it('keeps Confirm disabled while the check runs', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get('*/api/appstore/plugins/:slug/return/check', async () => {
        await held;
        return HttpResponse.json({ success: true, data: CLEAR });
      }),
    );
    mount();
    expect(await screen.findByText('Checking whether Alpha can return to stable...')).toBeInTheDocument();
    expectBlocked(await confirm());
    release();
    await waitFor(async () => expect(await confirm()).not.toHaveAttribute('aria-disabled', 'true'));
  });

  it('shows the server message when the check fails, disabled, and checks again on request', async () => {
    checkStatus = 500;
    const user = userEvent.setup();
    mount();
    expect(await screen.findByText('The catalog could not be read: connection reset')).toBeInTheDocument();
    expectBlocked(await confirm());
    expect(checks).toHaveBeenCalledTimes(1);
    checkStatus = 200;
    await user.click(screen.getByRole('button', { name: 'Check again' }));
    await waitFor(async () => expect(await confirm()).not.toHaveAttribute('aria-disabled', 'true'));
    expect(checks).toHaveBeenCalledTimes(2);
  });
});

describe('PluginReturnDialog live regions and focus', () => {
  function regions() {
    const dialog = screen.getByRole('dialog');
    return { status: within(dialog).getByRole('status'), alert: within(dialog).getByRole('alert') };
  }

  it('mounts one status and one alert region from the first render and swaps content between them', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get('*/api/appstore/plugins/:slug/return/check', async () => {
        await held;
        return HttpResponse.json({ success: true, data: { ok: false, outcome: 'blocked', blocking_migration: '003_x.sql' } });
      }),
    );
    mount();
    await screen.findByRole('dialog');
    const first = regions();
    expect(first.status).toHaveTextContent('Checking whether Alpha can return to stable');
    expect(first.alert).toBeEmptyDOMElement();
    release();
    await waitFor(() => expect(first.alert).toHaveTextContent('003_x.sql'));
    const after = regions();
    expect(after.alert).toBe(first.alert);
    expect(after.status).toBe(first.status);
    expect(after.status.textContent).toBe('');
  });

  it('keeps the alert empty when the return is ready, and puts the text in the status', async () => {
    mount();
    await ready();
    const { status, alert } = regions();
    expect(status).toHaveTextContent('Ready to return. Nothing else blocks it.');
    expect(alert).toBeEmptyDOMElement();
  });

  it('moves focus to the alert region when a block first appears', async () => {
    setup({ ok: false, outcome: 'blocked', blocking_migration: '003_x.sql' });
    mount();
    await waitFor(() => expect(regions().alert).toHaveTextContent('003_x.sql'));
    await waitFor(() => expect(regions().alert).toHaveFocus());
  });

  it('puts a refusal in the alert region and focuses it', async () => {
    server.use(
      http.post('*/api/appstore/plugins/:slug/return', () =>
        HttpResponse.json({ success: false, error: 'Blocked by 003_x.sql', code: 'e' }, { status: 409 }),
      ),
    );
    const user = userEvent.setup();
    mount();
    await user.click(await ready());
    await waitFor(() => expect(regions().alert).toHaveTextContent('Blocked by 003_x.sql'));
    await waitFor(() => expect(regions().alert).toHaveFocus());
    expect(regions().status.textContent).toBe('');
  });

  it('describes a disabled Confirm by the blocking reason, keeps it focusable and refuses activation', async () => {
    setup({ ok: false, outcome: 'blocked', blocking_migration: '003_x.sql' });
    const user = userEvent.setup();
    mount();
    const button = await confirm();
    await waitFor(() => expect(button).toHaveAccessibleDescription(/003_x\.sql/));
    expect(button).not.toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    button.focus();
    expect(button).toHaveFocus();
    await user.click(button);
    expect(returns).not.toHaveBeenCalled();
  });

  it('prints a failed check message once, in the alert, with the error icon outside the live region', async () => {
    checkStatus = 500;
    mount();
    const message = 'The catalog could not be read: connection reset';
    await waitFor(() => expect(regions().alert).toHaveTextContent(message));
    expect(screen.getAllByText(message)).toHaveLength(1);
    expect(within(regions().alert).queryByRole('button')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'View the error' })).toBeInTheDocument();
  });
});

describe('PluginReturnDialog unsigned package', () => {
  beforeEach(() => setup({ ok: false, outcome: 'unsigned', unsigned: true }));

  it('warns, keeps Confirm disabled until consent is ticked, and sends allow_unsigned only then', async () => {
    const user = userEvent.setup();
    mount();
    expect(await screen.findByText(/This package has no signature/)).toBeInTheDocument();
    const button = await confirm();
    expectBlocked(button);
    await user.click(button);
    expect(returns).not.toHaveBeenCalled();
    await user.click(screen.getByRole('checkbox', { name: /unsigned package/ }));
    expect(await screen.findByText('Consent given. Confirm to continue.')).toBeInTheDocument();
    await user.click(await ready());
    await waitFor(() => expect(returns).toHaveBeenCalledTimes(1));
    expect(returns).toHaveBeenCalledWith({ allow_unsigned: true });
  });

  it('unticks consent when the server then refuses', async () => {
    server.use(
      http.post('*/api/appstore/plugins/:slug/return', () =>
        HttpResponse.json({ success: false, error: 'Blocked by 003_x.sql', code: 'e' }, { status: 409 }),
      ),
    );
    const user = userEvent.setup();
    mount();
    const box = await screen.findByRole('checkbox', { name: /unsigned package/ });
    await user.click(box);
    await user.click(await ready());
    expect(await screen.findByText('Blocked by 003_x.sql')).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /unsigned package/ })).not.toBeChecked();
  });
});
