// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import i18n from 'i18next';
import { http, HttpResponse } from 'msw';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { RunbookEditorDialog } from './RunbookEditorDialog';
import type { RunbookCatalog, RunbookInput } from '@resources/types/runbooks';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const catalog: RunbookCatalog = {
  actions: [
    {
      key: 'vm.start',
      description: 'Start the virtual machine',
      paramSpecs: [{ name: 'vm', type: 'target', target: 'vm', required: false }],
      requiredPermissions: ['vm.manage'],
      destructive: true,
      streams: false,
      plugin: 'addon-kvm',
      target: 'vm',
    },
  ],
  diagnostics: [],
  targets: [{ kind: 'vm', labelKey: 'runbook.targets.vm', labelNs: 'vm' }],
  attachKinds: [{ kind: 'vm', labelKey: 'runbook.attach.vm', labelNs: 'vm', selector: 'vm' }],
};

const widgetCatalog: RunbookCatalog = {
  actions: [
    {
      key: 'widget.spin',
      description: '',
      descriptionKey: 'runbook.steps.spin',
      descriptionNs: 'widgets',
      paramSpecs: [{ name: 'unit', type: 'target', target: 'widget_unit', required: false }],
      requiredPermissions: ['widgets.manage'],
      destructive: false,
      streams: false,
      plugin: 'addon-widgets',
      target: 'widget_unit',
    },
  ],
  diagnostics: [],
  targets: [{ kind: 'widget_unit', labelKey: 'runbook.targets.widget_unit', labelNs: 'widgets' }],
};

beforeAll(() => {
  i18n.addResourceBundle(
    'en',
    'vm',
    { runbook: { targets: { vm: 'Virtual machine' }, attach: { vm: 'Virtual machine' } } },
    true,
    true,
  );
  i18n.addResourceBundle(
    'en',
    'widgets',
    { runbook: { targets: { widget_unit: 'Widget unit' }, steps: { spin: 'Spin the unit' } } },
    true,
    true,
  );
});

function backend(bodies: RunbookInput[] = []) {
  server.use(
    http.get('*/api/runbooks/catalog', () => HttpResponse.json({ success: true, data: catalog })),
    http.get('*/api/runbooks/targets', ({ request }) => {
      const kind = new URL(request.url).searchParams.get('kind');
      const data = kind === 'vm' ? [{ id: 'v-123', name: 'web-vm', detail: 'running' }] : [];
      return HttpResponse.json({ success: true, data });
    }),
    http.get('*/api/environments', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/alerts/conditions', () => HttpResponse.json({ success: true, data: { sources: [] } })),
    http.post('*/api/runbooks', async ({ request }) => {
      bodies.push((await request.json()) as RunbookInput);
      return HttpResponse.json({ success: true, data: { id: 'rb-new' } }, { status: 201 });
    }),
  );
}

async function pick(user: ReturnType<typeof userEvent.setup>, trigger: string, option: RegExp) {
  await user.click(await screen.findByRole('button', { name: trigger }));
  await user.click(await screen.findByRole('button', { name: option }));
}

describe('RunbookEditorDialog', () => {
  it('asks before discarding unsaved changes, and closes at once when nothing changed', async () => {
    backend();
    grantPermissions('*');
    const onClose = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<RunbookEditorDialog open runbookId={null} onClose={onClose} />);

    const dialog = await screen.findByRole('dialog', { name: 'Create runbook' });
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByText('Discard your changes?')).not.toBeInTheDocument();

    await user.type(within(dialog).getByRole('textbox', { name: 'Name' }), 'Disk full');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(await screen.findByText('Discard your changes?')).toBeInTheDocument();
    expect(onClose).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it('sends the ids chosen in the selectors, not their labels', async () => {
    const bodies: RunbookInput[] = [];
    backend(bodies);
    grantPermissions('*');
    const onClose = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<RunbookEditorDialog open runbookId={null} onClose={onClose} />);

    const dialog = await screen.findByRole('dialog', { name: 'Create runbook' });
    await user.type(within(dialog).getByRole('textbox', { name: 'Name' }), 'VM hung');
    await pick(user, 'Attach to', /Virtual machine/);
    await pick(user, 'What it is attached to', /web-vm/);
    await user.type(within(dialog).getByRole('textbox', { name: 'Step title' }), 'Start it');
    await pick(user, 'Step kind', /^Action$/);
    await pick(user, 'Action', /vm\.start/);
    await pick(user, 'Virtual machine', /web-vm/);
    await user.click(within(dialog).getByRole('button', { name: 'Save runbook' }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({ attachKind: 'vm', attachSubject: 'v-123' });
    expect(bodies[0]?.steps[0]).toMatchObject({ kind: 'action', actionKey: 'vm.start', actionParams: { vm: 'v-123' } });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('words a plugin step from the plugin namespace, not from the host', async () => {
    i18n.addResourceBundle('en', 'vm', { runbook: { steps: { vm_start: 'Boot the guest' } } }, true, true);
    backend();
    const [entry] = catalog.actions;
    server.use(
      http.get('*/api/runbooks/catalog', () =>
        HttpResponse.json({
          success: true,
          data: {
            ...catalog,
            actions: [{ ...entry, description: '', descriptionKey: 'runbook.steps.vm_start', descriptionNs: 'vm' }],
          },
        }),
      ),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(<RunbookEditorDialog open runbookId={null} onClose={vi.fn()} />);

    const dialog = await screen.findByRole('dialog', { name: 'Create runbook' });
    await pick(user, 'Step kind', /^Action$/);
    await pick(user, 'Action', /vm\.start/);
    expect(await within(dialog).findByText(/Boot the guest/)).toBeInTheDocument();
    expect(within(dialog).queryByText(/runbook\.steps\.vm_start/)).not.toBeInTheDocument();
  });

  it('renders a target kind the host has never heard of, titled and listed from what the server reports', async () => {
    backend();
    server.use(
      http.get('*/api/runbooks/catalog', () => HttpResponse.json({ success: true, data: widgetCatalog })),
      http.get('*/api/runbooks/targets', ({ request }) => {
        const kind = new URL(request.url).searchParams.get('kind');
        const data = kind === 'widget_unit' ? [{ id: 'u-7', name: 'spinner-7' }] : [];
        return HttpResponse.json({ success: true, data });
      }),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(<RunbookEditorDialog open runbookId={null} onClose={vi.fn()} />);

    const dialog = await screen.findByRole('dialog', { name: 'Create runbook' });
    await pick(user, 'Step kind', /^Action$/);
    await pick(user, 'Action', /widget.spin/);
    await pick(user, 'Widget unit', /spinner-7/);
    expect(within(dialog).queryByText(/runbook.targets.widget_unit|param.unit/)).not.toBeInTheDocument();
  });

  it('offers a plugin attach target only while its plugin is loaded', async () => {
    backend();
    server.use(
      http.get('*/api/runbooks/catalog', () =>
        HttpResponse.json({ success: true, data: { ...catalog, actions: [], targets: [], attachKinds: [] } }),
      ),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(<RunbookEditorDialog open runbookId={null} onClose={vi.fn()} />);

    await screen.findByRole('dialog', { name: 'Create runbook' });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Attach to' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Attach to' }));
    expect(await screen.findByRole('button', { name: /^Stack$/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Virtual machine/ })).not.toBeInTheDocument();
  });

  it('says the provider is not installed for a stored attach kind no loaded plugin declares', async () => {
    backend();
    server.use(
      http.get('*/api/runbooks/catalog', () =>
        HttpResponse.json({ success: true, data: { ...catalog, actions: [], targets: [], attachKinds: [] } }),
      ),
      http.get('*/api/runbooks/principals', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/runbooks/rb-orphan', () =>
        HttpResponse.json({
          success: true,
          data: {
            id: 'rb-orphan',
            name: 'Guest hung',
            description: '',
            attachKind: 'vm',
            attachSubject: 'v-9',
            enabled: true,
            steps: [],
            createdAt: '2026-09-30T10:00:00Z',
            updatedAt: '2026-09-30T10:00:00Z',
          },
        }),
      ),
    );
    grantPermissions('*');
    renderWithProviders(<RunbookEditorDialog open runbookId="rb-orphan" onClose={vi.fn()} />);

    const trigger = await screen.findByRole('button', { name: 'Attach to' });
    await waitFor(() => expect(trigger).toHaveTextContent('Provider not installed: vm'));
    expect(await screen.findByRole('alert')).toHaveTextContent('is not installed');
  });
});
