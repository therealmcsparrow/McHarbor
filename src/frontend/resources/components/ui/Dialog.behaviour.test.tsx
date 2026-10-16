// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it, vi } from 'vitest';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@resources/components/ui/Dialog';

function Example({
  hideCloseButton = false,
  onOpenChange,
}: {
  hideCloseButton?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  return (
    <Dialog onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        <Button>Open settings</Button>
      </DialogTrigger>
      <DialogContent hideCloseButton={hideCloseButton}>
        <DialogHeader>
          <DialogTitle>Rename volume</DialogTitle>
          <DialogDescription>The new name applies on the next mount.</DialogDescription>
        </DialogHeader>
        <input aria-label="Name" />
      </DialogContent>
    </Dialog>
  );
}

describe('Dialog', () => {
  it('opens from its trigger with an accessible name and description', async () => {
    const user = userEvent.setup();
    render(<Example />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Open settings' }));
    const dialog = await screen.findByRole('dialog', { name: 'Rename volume' });
    expect(dialog).toHaveAccessibleDescription('The new name applies on the next mount.');
  });

  it('moves focus inside when it opens', async () => {
    const user = userEvent.setup();
    render(<Example />);
    await user.click(screen.getByRole('button', { name: 'Open settings' }));
    const dialog = await screen.findByRole('dialog');
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement));
  });

  it('closes on Escape, reports it, and returns focus to the trigger', async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    render(<Example onOpenChange={onOpenChange} />);
    const trigger = screen.getByRole('button', { name: 'Open settings' });
    await user.click(trigger);
    await screen.findByRole('dialog');
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(onOpenChange).toHaveBeenLastCalledWith(false);
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('closes from its translated close button', async () => {
    const user = userEvent.setup();
    render(<Example />);
    await user.click(screen.getByRole('button', { name: 'Open settings' }));
    await user.click(await screen.findByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('omits the close button when asked to', async () => {
    const user = userEvent.setup();
    render(<Example hideCloseButton />);
    await user.click(screen.getByRole('button', { name: 'Open settings' }));
    await screen.findByRole('dialog');
    expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
  });

  it('renders on a solid popover surface, not the glass or aurora gradient', async () => {
    const user = userEvent.setup();
    render(<Example />);
    await user.click(screen.getByRole('button', { name: 'Open settings' }));
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveClass('bg-popover', 'border', 'border-border', 'text-popover-foreground');
    const classes = Array.from(dialog.classList);
    expect(classes.filter((name) => name.startsWith('mc-glass'))).toEqual([]);
    expect(classes.filter((name) => /gradient|aurora|mc-menu|bg-glass|backdrop-blur/.test(name))).toEqual([]);
  });

  it('keeps the modal class free of a background that would beat bg-popover', () => {
    const css = readFileSync(resolve(__dirname, '../../../app.css'), 'utf8');
    const rules = Array.from(css.matchAll(/(^|\n)([^{}\n]*\.mc-modal\b[^{}]*)\{([^}]*)\}/g));
    expect(rules.length).toBeGreaterThan(0);
    for (const [, , , body] of rules) {
      expect(body).not.toMatch(/background|gradient|aurora|glass-solid/);
    }
  });
});
