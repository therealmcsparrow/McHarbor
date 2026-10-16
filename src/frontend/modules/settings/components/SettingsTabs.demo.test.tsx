// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import { DEMO_STATUS_KEY } from '@resources/hooks/useDemoMode';
import type { DemoStatus } from '@resources/types/demo';
import { DEMO_STATUS_SIZES } from '../demo/test-fixtures';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { SettingsTabs } from './SettingsTabs';
import { useHiddenSettingsTabs } from './useHiddenSettingsTabs';

function Harness() {
  const hiddenTabs = useHiddenSettingsTabs();
  return (
    <SettingsTabs
      activeTab="about"
      onTabChange={() => undefined}
      hiddenTabs={hiddenTabs}
      labelFor={(tab) => `section ${tab}`}
      groupLabelFor={(key) => key}
      ariaLabel="sections"
    />
  );
}

function serve(available: boolean, active: boolean) {
  const status: DemoStatus = {
    available,
    active,
    databaseExists: false,
    clustered: false,
    canEnter: !active,
    ...DEMO_STATUS_SIZES,
  };
  server.use(http.get('*/api/demo/status', () => HttpResponse.json({ success: true, data: status })));
}

describe('Settings → Demo mode visibility', () => {
  it('leaves the section out when no demo_mode file is in the root', async () => {
    serve(false, false);
    const { client } = renderWithProviders(
      <MemoryRouter>
        <Harness />
      </MemoryRouter>,
    );
    // Hidden while loading too, so wait for the answer before judging it.
    await waitFor(() => expect(client.getQueryData(DEMO_STATUS_KEY)).toBeDefined());
    expect(await screen.findByRole('tab', { name: /section about/ })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: /section demo/ })).not.toBeInTheDocument();
  });

  it('shows the section when the marker file is present', async () => {
    serve(true, false);
    renderWithProviders(
      <MemoryRouter>
        <Harness />
      </MemoryRouter>,
    );
    expect(await screen.findByRole('tab', { name: /section demo/ })).toBeInTheDocument();
  });

  it('keeps the section while a demo runs, so it can always be left', async () => {
    serve(false, true);
    renderWithProviders(
      <MemoryRouter>
        <Harness />
      </MemoryRouter>,
    );
    expect(await screen.findByRole('tab', { name: /section demo/ })).toBeInTheDocument();
  });
});
