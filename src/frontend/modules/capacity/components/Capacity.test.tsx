// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { CapacityOverview } from '@resources/components/capacity/CapacityOverview';
import { IdleResources } from '@resources/components/capacity/IdleResources';
import type { CapacityReport, CapacitySubject, IdleReport } from '@resources/hooks/useCapacity';

const climbing: CapacitySubject = {
  environmentId: 'e1',
  subjectKind: 'rootfs',
  subjectId: 'e1',
  subjectName: '/',
  usedBytes: 87_000_000_000,
  totalBytes: 100_000_000_000,
  usedPercent: 87,
  observedDays: 12,
  requiredDays: 7,
  samples: 288,
  windowDays: 30,
  percentPerDay: 1,
  r2: 0.98,
  red: { percent: 90, days: 3, at: '2026-10-06T00:00:00Z' },
};

const dated: CapacityReport = {
  windowDays: 30,
  requiredDays: 7,
  amberPercent: 75,
  redPercent: 90,
  bucketHours: 1,
  subjects: [climbing],
};

const refused: CapacityReport = {
  ...dated,
  subjects: [
    {
      ...climbing,
      observedDays: 3,
      samples: 72,
      percentPerDay: 0,
      r2: 0,
      red: undefined,
      refusal: 'short_series',
    },
  ],
};

function serveCapacity(report: CapacityReport) {
  server.use(http.get('*/api/metrics/capacity', () => HttpResponse.json({ success: true, data: report })));
}

function serveIdle(report: IdleReport) {
  server.use(http.get('*/api/metrics/capacity/idle', () => HttpResponse.json({ success: true, data: report })));
}

describe('Capacity forecast surface', () => {
  beforeEach(() => {
    grantPermissions(['host.view']);
  });

  it('states the date and the evidence it came from', async () => {
    serveCapacity(dated);
    renderWithProviders(<CapacityOverview envId="e1" />);

    expect(await screen.findByText(/reaches 90% on (6 October|October 6)/i)).toBeInTheDocument();
    expect(screen.getByText(/R² 0\.98/)).toBeInTheDocument();
    expect(screen.getByText(/over 12 days/)).toBeInTheDocument();
    expect(screen.getByText(/last 30 days/)).toBeInTheDocument();
  });

  it('says what is missing instead of extrapolating', async () => {
    serveCapacity(refused);
    renderWithProviders(<CapacityOverview envId="e1" />);

    expect(await screen.findByText(/needs 7 days, has 3/i)).toBeInTheDocument();
    expect(screen.queryByText(/reaches 90% on/i)).not.toBeInTheDocument();
  });
});

describe('Idle resources', () => {
  beforeEach(() => {
    grantPermissions(['host.view']);
  });

  it('renders an unknown size as words, never as 0 B', async () => {
    serveIdle({
      thresholdDays: 14,
      reclaimableBytes: 0,
      sizesUnavailable: true,
      containers: [
        {
          environmentId: 'e1',
          containerId: 'c1',
          containerName: 'old-api',
          reason: 'stopped',
          idleDays: 61,
          sizeKnown: false,
          sizeReason: 'agent_environment',
          sizeRwBytes: 0,
          sizeRootBytes: 0,
        },
      ],
    });
    renderWithProviders(<IdleResources envId="e1" />);

    expect(await screen.findByText('old-api')).toBeInTheDocument();
    expect(screen.getByText(/Size unknown — not collected for agent environments/i)).toBeInTheDocument();
    expect(screen.queryByText('0 B')).not.toBeInTheDocument();
  });

  it('renders a known size as a size', async () => {
    serveIdle({
      thresholdDays: 14,
      reclaimableBytes: 1024,
      sizesUnavailable: false,
      containers: [
        {
          environmentId: 'e1',
          containerId: 'c2',
          containerName: 'old-worker',
          reason: 'stopped',
          idleDays: 30,
          sizeKnown: true,
          sizeReason: undefined,
          sizeRwBytes: 5_242_880,
          sizeRootBytes: 10_485_760,
        },
      ],
    });
    renderWithProviders(<IdleResources envId="e1" />);

    expect(await screen.findByText('old-worker')).toBeInTheDocument();
    expect(screen.getByText('5 MB')).toBeInTheDocument();
    expect(screen.queryByText(/Size unknown/i)).not.toBeInTheDocument();
  });

  it('labels the weaker signal differently', async () => {
    serveIdle({
      thresholdDays: 14,
      reclaimableBytes: 0,
      sizesUnavailable: false,
      containers: [
        {
          environmentId: 'e1',
          containerId: 'c3',
          containerName: 'sleepy',
          reason: 'quiet',
          idleDays: 20,
          sizeKnown: true,
          sizeRwBytes: 1024,
          sizeRootBytes: 2048,
        },
      ],
    });
    renderWithProviders(<IdleResources envId="e1" />);

    expect(await screen.findByText('No traffic')).toBeInTheDocument();
    expect(screen.queryByText('Stopped')).not.toBeInTheDocument();
  });
});
