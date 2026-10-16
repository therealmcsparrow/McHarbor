// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { ImageInfo } from '@core/types/docker';
import { useImagesPageConfig } from './useImagesPageConfig';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';

const image = { Id: 'sha256:abc', RepoTags: ['app:1'], Labels: {}, Containers: 0 } as unknown as ImageInfo;

type CellRenderer = (ctx: { row: { original: ImageInfo } }) => ReactNode;

function ActionsCell({ onExport }: { onExport: (image: ImageInfo) => void }) {
  const { columns } = useImagesPageConfig({ onExport, onRemove: vi.fn(), onBatchRemove: vi.fn() });
  const actions = columns.find((column) => column.id === 'actions');
  const cell = actions?.cell as unknown as CellRenderer;
  return <>{cell({ row: { original: image } })}</>;
}

describe('image row export', () => {
  it('renders Export disabled, naming images.manage, and exports nothing when denied', async () => {
    grantPermissions(['images.view']);
    const onExport = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<ActionsCell onExport={onExport} />);
    const button = screen.getByRole('button', { name: 'Export Image' });
    expect(button).toBeDisabled();
    await user.click(button);
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the images.manage permission')).not.toHaveLength(0);
    expect(onExport).not.toHaveBeenCalled();
  });

  it('exports the row when images.manage is held', async () => {
    grantPermissions(['images.view', 'images.manage']);
    const onExport = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<ActionsCell onExport={onExport} />);
    await user.click(screen.getByRole('button', { name: 'Export Image' }));
    expect(onExport).toHaveBeenCalledWith(image);
  });
});
