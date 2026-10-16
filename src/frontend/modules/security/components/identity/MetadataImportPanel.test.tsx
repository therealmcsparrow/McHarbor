// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { MetadataImportPanel } from './MetadataImportPanel';

const result = {
  source: 'url',
  metadata: {
    entityId: 'https://idp.example.com/entity',
    ssoServices: [
      { binding: 'urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST', location: 'https://idp.example.com/sso' },
    ],
    sloServices: [],
    nameIdFormats: [],
    certificates: [{ use: 'signing', fingerprintSha256: 'AB:CD', pem: 'x' }],
    wantAuthnRequestsSigned: false,
  },
  prefill: { metadata_url: 'https://idp.example.com/md', unknown_key: 'nope' },
};

describe('MetadataImportPanel', () => {
  it('prefills only the fields the plugin form declares and shows the rest to copy', async () => {
    grantPermissions(['identity_providers.manage']);
    const bodies: unknown[] = [];
    server.use(
      http.post('*/api/identity-providers/metadata-import', async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json({ success: true, data: result });
      }),
    );
    const onPrefill = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <MetadataImportPanel schema={[{ key: 'metadata_url' }, { key: 'entity_id' }]} onPrefill={onPrefill} />,
    );
    await user.type(screen.getByLabelText('Metadata URL'), 'https://idp.example.com/md');
    await user.click(screen.getByRole('button', { name: 'Read metadata' }));
    expect(await screen.findByText('https://idp.example.com/entity')).toBeInTheDocument();
    expect(screen.getByText('AB:CD')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Fill in the form' }));
    expect(onPrefill).toHaveBeenCalledWith({ metadata_url: 'https://idp.example.com/md' });
    expect(bodies).toEqual([{ url: 'https://idp.example.com/md' }]);
  });

  it('explains why an import failed', async () => {
    grantPermissions(['identity_providers.manage']);
    server.use(
      http.post('*/api/identity-providers/metadata-import', () =>
        HttpResponse.json(
          {
            success: false,
            error: 'Invalid request body',
            code: 'err.invalid_body',
            data: { reason: 'no_idp_descriptor' },
          },
          { status: 400 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<MetadataImportPanel schema={[]} onPrefill={vi.fn()} />);
    await user.type(screen.getByLabelText('Or paste the metadata XML'), 'x');
    await user.click(screen.getByRole('button', { name: 'Read metadata' }));
    expect(
      await screen.findByText('The metadata describes no identity provider (no IDPSSODescriptor).'),
    ).toBeInTheDocument();
  });

  it('renders the import disabled, naming identity_providers.manage, without the key', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<MetadataImportPanel schema={[]} onPrefill={vi.fn()} />);
    await user.type(screen.getByLabelText('Metadata URL'), 'https://idp.example.com/md');
    const button = screen.getByRole('button', { name: 'Read metadata' });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the identity_providers.manage permission')).not.toHaveLength(0);
  });
});
