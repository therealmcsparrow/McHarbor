// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { AlertTemplateField } from './AlertTemplateField';

const reference = {
  variables: [
    { name: 'subject', description: 'x', sample: 'web-1' },
    { name: 'value', description: 'x', sample: '91.2%' },
  ],
  context: [
    { key: 'used_bytes', description: 'x', learned: false },
    { key: 'vm_host', description: '', learned: true },
  ],
};

function Harness({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial);
  return (
    <AlertTemplateField
      value={value}
      onChange={setValue}
      name="Disk"
      severity="warning"
      sourceType="host"
      conditionKey="docker_disk"
      condition={{ operator: '>', threshold: 85 }}
    />
  );
}

describe('Alert template field', () => {
  let previews: { template: string; state: string; conditionKey: string }[];

  beforeEach(() => {
    previews = [];
    grantPermissions(['alerts.view', 'alerts.manage']);
    server.use(
      http.get('*/api/alerts/template-reference', ({ request }) => {
        const url = new URL(request.url);
        expect(url.searchParams.get('condition')).toBe('docker_disk');
        return HttpResponse.json({ success: true, data: reference });
      }),
      http.post('*/api/alerts/template-preview', async ({ request }) => {
        const body = (await request.json()) as { template: string; state: string; conditionKey: string };
        previews.push(body);
        return HttpResponse.json({
          success: true,
          data: {
            message: `rendered:${body.template}:${body.state}`,
            unknown: body.template.includes('{{valeu}}') ? ['{{valeu}}'] : [],
          },
        });
      }),
    );
  });

  it('inserts variables and condition keys at the cursor and previews through the server', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness initial="" />);

    const box = screen.getByRole('textbox', { name: 'Message template' }) as HTMLTextAreaElement;
    await user.click(await screen.findByRole('button', { name: /\{\{subject\}\}/ }));
    await user.type(box, ' at ');
    await user.click(screen.getByRole('tab', { name: /Condition/ }));
    expect(screen.getByText("Seen on this condition's recent evaluations")).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /\{\{ctx\.used_bytes\}\}/ }));
    expect(box.value).toBe('{{subject}} at {{ctx.used_bytes}}');

    await waitFor(() =>
      expect(screen.getByTestId('alert-template-preview')).toHaveTextContent(
        'rendered:{{subject}} at {{ctx.used_bytes}}:firing',
      ),
    );
    await user.click(screen.getByRole('button', { name: 'Resolved' }));
    await waitFor(() => expect(screen.getByTestId('alert-template-preview')).toHaveTextContent(':resolved'));
    expect(previews.at(-1)?.conditionKey).toBe('docker_disk');
  });

  it('inserts in the middle of existing text, where the cursor is', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness initial="Disk: now" />);
    const box = screen.getByRole('textbox', { name: 'Message template' }) as HTMLTextAreaElement;
    box.focus();
    box.setSelectionRange(6, 6);
    await user.click(await screen.findByRole('button', { name: /\{\{value\}\}/ }));
    expect(box.value).toBe('Disk: {{value}}now');
  });

  it('warns about placeholders the renderer does not know', async () => {
    renderWithProviders(<Harness initial="{{valeu}}" />);
    expect(await screen.findByText('Not recognised, sent as written: {{valeu}}')).toBeInTheDocument();
  });

  it('replaces the template with a chosen example', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness initial="old" />);
    await user.click(screen.getByRole('tab', { name: 'Examples' }));
    await user.click(screen.getByRole('button', { name: /Ticket friendly/ }));
    expect((screen.getByRole('textbox', { name: 'Message template' }) as HTMLTextAreaElement).value).toBe(
      '{{alert}} | {{environment}}/{{subject}} | {{rule}} | observed {{value}} | {{timestamp}}',
    );
  });
});
