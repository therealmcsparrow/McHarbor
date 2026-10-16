// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { FileEditorDialog } from './FileEditorDialog';

vi.mock('@resources/components/CodeEditor', () => ({
  CodeEditor: ({ value, onChange }: { value: string; onChange: (value: string) => void }) => (
    <textarea aria-label="editor" value={value} onChange={(event) => onChange(event.target.value)} />
  ),
}));

const DISCARD_TITLE = 'Discard unsaved changes?';

async function mount() {
  const onOpenChange = vi.fn();
  renderWithProviders(<FileEditorDialog open onOpenChange={onOpenChange} containerId="c1" filePath="/etc/app.yml" />);
  const editor = await screen.findByLabelText('editor');
  await waitFor(() => expect(editor).toHaveValue('a: 1'));
  return { onOpenChange, editor };
}

describe('FileEditorDialog unsaved changes', () => {
  beforeEach(() => {
    grantPermissions([]);
    server.use(http.get('*/api/containers/c1/files/content', () => new HttpResponse('a: 1')));
  });

  it('closes an untouched editor without asking', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = await mount();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(screen.queryByText(DISCARD_TITLE)).not.toBeInTheDocument();
  });

  it('asks before closing an edited file and keeps the editor open when the answer is no', async () => {
    const user = userEvent.setup();
    const { onOpenChange, editor } = await mount();
    await user.type(editor, ' # edited');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    const confirm = await screen.findByRole('dialog', { name: DISCARD_TITLE });
    expect(onOpenChange).not.toHaveBeenCalled();
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByText(DISCARD_TITLE)).not.toBeInTheDocument());
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByLabelText('editor')).toHaveValue('a: 1 # edited');
  });

  it('closes the editor once, and only after the discard is confirmed', async () => {
    const user = userEvent.setup();
    const { onOpenChange, editor } = await mount();
    await user.type(editor, ' # edited');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    const confirm = await screen.findByRole('dialog', { name: DISCARD_TITLE });
    expect(within(confirm).getByText('You have unsaved changes. Discard?')).toBeInTheDocument();
    await user.click(within(confirm).getByRole('button', { name: 'Discard' }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(onOpenChange).toHaveBeenCalledTimes(1);
  });
});
