// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { PERM } from '@core/auth/permissionKeys';
import type { AppTemplate } from '../types';
import { InstallDialog } from './InstallDialog';

const env = { id: 'env-1', name: 'Local', connectionType: 'socket' };

const app: AppTemplate = {
  id: 'a1',
  slug: 'acme-app',
  name: 'Acme App',
  description: '',
  category: 'Tools',
  image: 'acme/app:latest',
  logo: '',
  website: '',
  docsUrl: '',
  ports: [],
  volumes: [],
  envVars: [],
  source: 'test',
  version: '1',
  updatedAt: '',
  installed: false,
  installations: [],
};

function sse(evt: object): Uint8Array {
  return new TextEncoder().encode(`data: ${JSON.stringify(evt)}\n\n`);
}

describe('InstallDialog', () => {
  beforeAll(() => {
    // jsdom has no layout; the progress log scrolls its last line into view.
    Element.prototype.scrollIntoView = vi.fn();
  });

  it('stays on the install progress when the environment list refreshes mid-install, and ends on Done', async () => {
    grantPermissions([PERM.STORE_APPS_MANAGE]);
    let push: ReadableStreamDefaultController<Uint8Array> | undefined;
    server.use(
      http.get('*/api/environments', () => HttpResponse.json({ success: true, data: [env] })),
      http.post(
        '*/api/app-store/install/stream',
        () =>
          new HttpResponse(
            new ReadableStream<Uint8Array>({
              start(controller) {
                push = controller;
                controller.enqueue(sse({ step: 1, total: 5, message: 'Pulling image', status: 'progress' }));
              },
            }),
            { headers: { 'Content-Type': 'text/event-stream' } },
          ),
      ),
    );

    const { client } = renderWithProviders(<InstallDialog app={app} open onOpenChange={() => {}} />);
    client.setQueryData(['environments'], [env]);
    const install = () => screen.getByRole('button', { name: 'Install with Defaults' });
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());
    expect(await screen.findByText('Pulling image')).toBeInTheDocument();

    // The 30-second environment refetch: any changed field (a status, a last-seen time) makes
    // it a new array; identical data is structurally shared and would not.
    await act(async () => {
      client.setQueryData(['environments'], [{ ...env, status: 'online' }]);
    });
    expect(screen.getByText('Pulling image')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Install with Defaults' })).not.toBeInTheDocument();

    await act(async () => {
      push?.enqueue(sse({ step: 5, total: 5, message: 'Installed', status: 'done' }));
      push?.close();
    });
    expect(await screen.findByRole('button', { name: 'Done' })).toBeInTheDocument();
  });
});
