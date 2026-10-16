// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { NODE_DEFINITION_MAP } from '@resources/nodes/registry';
import type { ScannerInfo } from '@resources/hooks/useScannerSettings';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { ScannerSelect } from './ScannerSelect';

const scanner = (over: Partial<ScannerInfo>): ScannerInfo => ({
  id: 'cfg-1',
  name: 'engine-a',
  label: 'Engine A (prod)',
  available: true,
  isDefault: false,
  ...over,
});

function serveScanners(scanners: ScannerInfo[]) {
  server.use(
    http.get('*/api/scans/scanners', () =>
      HttpResponse.json({ success: true, data: { scanners, defaultScanner: '', defaultScannerId: '' } }),
    ),
  );
}

describe('ScannerSelect', () => {
  afterEach(() => resetPermissions());

  it('offers the scanners the installed plugins registered, and nothing the host names itself', async () => {
    serveScanners([scanner({}), scanner({ id: 'cfg-2', name: 'engine-b', label: 'Engine B', available: false })]);
    grantPermissions(['scans.view']);
    const onChange = vi.fn();
    renderWithProviders(<ScannerSelect value="" onChange={onChange} />);

    await userEvent.click(screen.getByRole('button', { name: 'Select scanner...' }));
    const offered = await screen.findByText('Engine A (prod)');
    expect(screen.queryByText('Engine B')).not.toBeInTheDocument();
    await userEvent.click(offered);

    expect(onChange).toHaveBeenCalledWith('cfg-1');
  });

  it('still names the scanner a workflow saved by engine type', async () => {
    serveScanners([scanner({})]);
    grantPermissions(['scans.view']);
    renderWithProviders(<ScannerSelect value="engine-a" onChange={vi.fn()} />);

    expect(await screen.findByText('Engine A (prod)')).toBeInTheDocument();
  });

  it('is disabled, saying why, when no scanner plugin is installed and configured', async () => {
    serveScanners([scanner({ available: false })]);
    grantPermissions(['scans.view']);
    renderWithProviders(<ScannerSelect value="" onChange={vi.fn()} />);

    expect(
      await screen.findByText('No scanner plugin is installed and configured. Add one under Security → Scanners.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Select scanner...' })).toBeDisabled();
  });

  it('is the field the image scan node uses, with no engine list of its own', () => {
    const field = NODE_DEFINITION_MAP['image-vulnerability-scan']?.configSchema.find((f) => f.key === 'scanner');
    expect(field?.type).toBe('image-scanner-select');
    expect(field?.options).toBeUndefined();
    expect(field?.default).toBeUndefined();
  });
});
