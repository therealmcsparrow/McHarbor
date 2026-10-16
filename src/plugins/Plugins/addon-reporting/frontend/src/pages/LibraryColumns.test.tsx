// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useLibraryColumns } from './LibraryColumns';
import type { StoredReport } from '../types';

function report(status: string): StoredReport {
  return {
    id: 'r1',
    report_type: 'monthly',
    title: 'Monthly estate review',
    environment_id: 'e1',
    environment: 'Production',
    period_start: '2026-09-01T00:00:00Z',
    period_end: '2026-09-30T00:00:00Z',
    levels: [],
    status,
    overall_score: 80,
    overall_band: 'strong',
    headline: 'All fine',
    generated_by: 'ops',
    size_bytes: 1,
    created_at: '2026-10-01T00:00:00Z',
  };
}

function ReportCell({ row }: { row: StoredReport }) {
  const columns = useLibraryColumns({ onDownload: () => undefined, onDelete: () => undefined });
  const column = columns.find((c) => c.id === 'report');
  const cell = column?.cell as (ctx: unknown) => React.ReactNode;
  return <>{cell({ row: { original: row } })}</>;
}

describe('library report column', () => {
  it('marks a report whose type was retired with a pill that explains why', () => {
    render(<ReportCell row={report('retired')} />);
    const pill = screen.getByText('Report type retired');
    expect(pill).toBeInTheDocument();
    expect(pill).toHaveAttribute('title', expect.stringMatching(/\S/));
    expect(screen.getByText('Monthly estate review')).toBeInTheDocument();
  });

  it('shows no pill for a report that can still be regenerated', () => {
    render(<ReportCell row={report('ready')} />);
    expect(screen.queryByText('Report type retired')).not.toBeInTheDocument();
    expect(screen.getByText('Monthly estate review')).toBeInTheDocument();
  });
});
