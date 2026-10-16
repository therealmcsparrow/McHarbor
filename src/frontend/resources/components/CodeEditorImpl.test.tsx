// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { CodeEditorImpl } from './CodeEditorImpl';

describe('CodeEditorImpl', () => {
  it('keeps one editor while its value changes, so typing never drops focus', () => {
    const { container, rerender } = render(<CodeEditorImpl value="a: 1" onChange={() => {}} />);
    const editor = container.querySelector('.cm-editor');
    expect(editor).not.toBeNull();

    // What a parent does on every keystroke: hand the new text straight back.
    rerender(<CodeEditorImpl value="a: 12" onChange={() => {}} />);

    expect(container.querySelector('.cm-editor')).toBe(editor);
    expect(container.querySelector('.cm-content')?.textContent).toBe('a: 12');
  });
});
