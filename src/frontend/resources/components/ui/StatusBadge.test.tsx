// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { CONTAINER_STATUS, ENVIRONMENT_STATUS, RECONCILER_STATUS, STACK_STATUS, StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
  it('uses mapped variants for known statuses', () => {
    render(<StatusBadge status="running" map={CONTAINER_STATUS} />);

    expect(screen.getByText('running')).toHaveClass('bg-success/15');
  });

  it('falls back to the secondary variant for unknown statuses', () => {
    render(<StatusBadge status="unknown" map={CONTAINER_STATUS} className="status-extra" />);

    expect(screen.getByText('unknown')).toHaveClass('bg-muted', 'status-extra');
  });

  it('exports standard status maps', () => {
    expect(CONTAINER_STATUS.exited).toBe('destructive');
    expect(STACK_STATUS.partial).toBe('warning');
    expect(ENVIRONMENT_STATUS.connected).toBe('success');
    expect(RECONCILER_STATUS.pending).toBe('secondary');
  });
});
