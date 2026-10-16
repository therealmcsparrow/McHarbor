// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { SecurityPostureCard } from './SecurityPostureCard';
import type { PostureFinding } from '../hooks/useSecurityPosture';

const requests = vi.fn();

function mount(findings: PostureFinding[]) {
  server.use(
    http.get('*/api/settings/security-posture', () => {
      requests();
      return HttpResponse.json({ success: true, data: { findings } });
    }),
  );
  return renderWithProviders(<SecurityPostureCard />);
}

describe('SecurityPostureCard', () => {
  beforeEach(() => {
    requests.mockReset();
    grantPermissions(['settings.view']);
  });

  it('lists each finding with its fix and the env var that addresses it', async () => {
    mount([
      { envVar: 'AUTH_DISABLE', severity: 'critical', message: 'authentication is disabled' },
      { envVar: 'HSTS_MAX_AGE', severity: 'warning', message: 'HSTS is off' },
    ]);
    expect(await screen.findByText('Authentication is disabled')).toBeInTheDocument();
    expect(screen.getByText('Critical')).toBeInTheDocument();
    expect(screen.getByText('AUTH_DISABLE')).toBeInTheDocument();
    expect(screen.getByText('HSTS is off')).toBeInTheDocument();
    expect(screen.getByText('Set HSTS_MAX_AGE=31536000 once browsers trust the certificate.')).toBeInTheDocument();
    expect(screen.getByText('HSTS_MAX_AGE')).toBeInTheDocument();
    expect(screen.queryByText('Every posture check passed.')).not.toBeInTheDocument();
  });

  it('shows the server message for a finding it has no copy for', async () => {
    mount([{ envVar: 'SOMETHING_NEW', severity: 'warning', message: 'a check added after this build' }]);
    expect(await screen.findByText('a check added after this build')).toBeInTheDocument();
    expect(screen.getByText('SOMETHING_NEW')).toBeInTheDocument();
  });

  it('says all clear when there are no findings', async () => {
    mount([]);
    expect(await screen.findByText('Every posture check passed.')).toBeInTheDocument();
    expect(screen.queryByText('Warning')).not.toBeInTheDocument();
  });

  it('names settings.view and asks for nothing without it', async () => {
    grantPermissions([]);
    mount([{ envVar: 'AUTH_DISABLE', severity: 'critical', message: 'authentication is disabled' }]);
    expect(await screen.findByText('Requires the settings.view permission')).toBeInTheDocument();
    expect(screen.queryByText('Authentication is disabled')).not.toBeInTheDocument();
    expect(requests).not.toHaveBeenCalled();
  });
});
