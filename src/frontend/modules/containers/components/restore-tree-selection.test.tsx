// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ContainerBackupRestoreOption } from '@resources/hooks/useContainerBackups';
import { renderWithProviders } from '../../../test/render';
import { coveredByFolder } from './restore-tree-keys';
import { applyEntryToggle } from './restore-tree-toggle';
import { TreeRow } from './RestoreTreeRow';

const option = (over: Partial<ContainerBackupRestoreOption>): ContainerBackupRestoreOption => ({
  key: '',
  type: 'file',
  label: '',
  description: '',
  default: false,
  required: false,
  ...over,
});

// The server's folder key once named the mount "mounts/data"; the restore reads
// "/data". The toggle must not trust it.
const htmlFolder = option({
  key: 'folder:mounts/data:html/',
  type: 'folder',
  label: 'html',
  path: 'html',
  parentMount: '/data',
});

describe('restore tree selection', () => {
  it('ticks a folder under the mount target the restore reads, whatever the server key says', () => {
    const next = applyEntryToggle({
      entry: htmlFolder,
      checked: true,
      kind: 'mount',
      effectiveMountTarget: '/data',
      selected: ['file:/data:html/index.php'],
      cache: new Map([['html', [option({ key: 'x', type: 'file', path: 'html/index.php', label: 'index.php' })]]]),
    });
    expect(next).toEqual(['folder:/data:html']);
  });

  it('treats everything under a ticked folder as selected, and nothing beside it', () => {
    const selected = ['folder:/data:html'];
    expect(coveredByFolder('html/index.php', '/data', selected)).toBe(true);
    expect(coveredByFolder('html/sub/a.txt', '/data', selected)).toBe(true);
    expect(coveredByFolder('html', '/data', selected)).toBe(false);
    expect(coveredByFolder('htmlx/a.txt', '/data', selected)).toBe(false);
    expect(coveredByFolder('html/index.php', '/other', selected)).toBe(false);
  });

  it('shows a file under a ticked folder as ticked and not separately togglable', () => {
    const onToggle = vi.fn();
    renderWithProviders(
      <TreeRow
        entry={option({ key: 'file:/data:html/index.php', type: 'file', label: 'index.php', path: 'html/index.php' })}
        mountTarget="/data"
        depth={1}
        selected={['folder:/data:html']}
        cache={new Map()}
        expanded={new Set()}
        onToggle={onToggle}
        onExpand={vi.fn()}
        formatSize={String}
        kind="mount"
        renderChildren={() => null}
      />,
    );
    const box = screen.getByRole('checkbox');
    expect(box).toBeChecked();
    expect(box).toBeDisabled();
    expect(box.parentElement).toHaveAttribute(
      'title',
      'Included with the folder above. Untick that folder to choose inside it.',
    );
  });
});
