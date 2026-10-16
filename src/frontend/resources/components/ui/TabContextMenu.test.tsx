// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { TabContextMenu } from '@resources/components/ui/Tabs';

function TabList({ onSelect }: { onSelect: () => void }) {
  return (
    <div role="tablist" aria-label="Sections">
      <TabContextMenu items={[{ id: 'pin', label: 'Pin', onSelect }]}>
        <button type="button" role="tab" aria-selected aria-controls="panel-a">
          Overview
        </button>
      </TabContextMenu>
    </div>
  );
}

describe('TabContextMenu', () => {
  it('leaves only tabs as meaningful children of the tablist', () => {
    render(<TabList onSelect={vi.fn()} />);
    const tab = screen.getByRole('tab', { name: 'Overview' });
    const wrapper = tab.parentElement as HTMLElement;
    expect(wrapper).toHaveAttribute('role', 'none');
    expect(wrapper).not.toHaveAttribute('aria-haspopup');
    expect(wrapper).not.toHaveAttribute('aria-controls');
  });

  it('announces the menu on the tab without replacing the panel it controls', () => {
    render(<TabList onSelect={vi.fn()} />);
    const tab = screen.getByRole('tab', { name: 'Overview' });
    expect(tab).toHaveAttribute('aria-haspopup', 'menu');
    expect(tab).toHaveAttribute('aria-controls', 'panel-a');
  });

  it('still opens the menu on right click', () => {
    const onSelect = vi.fn();
    render(<TabList onSelect={onSelect} />);
    fireEvent.contextMenu(screen.getByRole('tab', { name: 'Overview' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Pin' }));
    expect(onSelect).toHaveBeenCalledTimes(1);
  });
});
