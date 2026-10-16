// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, type RefObject } from 'react';

const MAX_ENTRIES = 50;

type UsePromptHistoryOptions = {
  /** Current input value — captured as the draft on the first recall. */
  value: string;
  setValue: (next: string) => void;
  /** Optional input element, used to park the caret at the end after recall. */
  inputRef?: RefObject<HTMLInputElement | null>;
};

/**
 * Shell-style prompt recall for a single-line input.
 *
 * Up walks back through submitted prompts, Down walks forward and lands on the
 * draft the user was typing before the first Up. History lives in refs: it is
 * never rendered, so changing it must not cost a render.
 */
export function usePromptHistory({ value, setValue, inputRef }: UsePromptHistoryOptions) {
  const entries = useRef<string[]>([]);
  // null = "not browsing history", the input holds the live draft.
  const cursor = useRef<number | null>(null);
  const draft = useRef('');

  const moveCaretToEnd = () => {
    const el = inputRef?.current;
    if (!el) return;
    // After a controlled re-render — otherwise we would measure the old value.
    requestAnimationFrame(() => {
      const end = el.value.length;
      el.setSelectionRange(end, end);
    });
  };

  /** Record a submitted prompt and leave history-browsing mode. */
  const remember = (entry: string) => {
    const trimmed = entry.trim();
    if (trimmed && entries.current[entries.current.length - 1] !== trimmed) {
      entries.current.push(trimmed);
      if (entries.current.length > MAX_ENTRIES) entries.current.shift();
    }
    cursor.current = null;
    draft.current = '';
  };

  /** Returns true when the key was consumed. */
  const recallPrevious = (): boolean => {
    const list = entries.current;
    if (list.length === 0) return false;
    if (cursor.current === null) {
      draft.current = value;
      cursor.current = list.length - 1;
    } else if (cursor.current > 0) {
      cursor.current -= 1;
    } else {
      // Already at the oldest entry — swallow the key so the caret stays put.
      return true;
    }
    setValue(list[cursor.current] ?? '');
    moveCaretToEnd();
    return true;
  };

  /** Returns true when the key was consumed. */
  const recallNext = (): boolean => {
    if (cursor.current === null) return false;
    const list = entries.current;
    const next = cursor.current + 1;
    if (next >= list.length) {
      cursor.current = null;
      setValue(draft.current);
    } else {
      cursor.current = next;
      setValue(list[next] ?? '');
    }
    moveCaretToEnd();
    return true;
  };

  /** Forget everything — used when the conversation is reset. */
  const clear = () => {
    entries.current = [];
    cursor.current = null;
    draft.current = '';
  };

  return { remember, recallPrevious, recallNext, clear };
}
