// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useCallback, useLayoutEffect, useRef } from 'react';
import type { EditFormData } from '../types/edit-form';
import type { ScrollSnapshot } from '../pages/container-detail-types';

/**
 * Captures the scroll position before an edit re-renders the panel and puts
 * it back afterwards, so typing in a long form does not jump the page.
 */
export function useContainerDetailScroll(edit: {
  editing: boolean;
  editData: unknown;
  onFieldChange: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
}) {
  const { editing, editData, onFieldChange } = edit;
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const pendingScrollRef = useRef<ScrollSnapshot | null>(null);
  const captureScroll = useCallback(() => {
    const main = document.querySelector('main');
    pendingScrollRef.current = {
      mainTop: main?.scrollTop ?? 0,
      paneTop: scrollContainerRef.current?.scrollTop ?? 0,
      windowTop: window.scrollY,
    };
  }, []);
  const restoreScroll = useCallback((snapshot: ScrollSnapshot) => {
    const main = document.querySelector('main');
    if (main) main.scrollTop = snapshot.mainTop;
    if (scrollContainerRef.current) scrollContainerRef.current.scrollTop = snapshot.paneTop;
    window.scrollTo({ top: snapshot.windowTop });
  }, []);
  useLayoutEffect(() => {
    const snapshot = pendingScrollRef.current;
    if (!snapshot) return;

    pendingScrollRef.current = null;
    restoreScroll(snapshot);
    const frame = requestAnimationFrame(() => restoreScroll(snapshot));
    return () => cancelAnimationFrame(frame);
  }, [editing, editData, restoreScroll]);
  const clearPendingScroll = useCallback(() => {
    pendingScrollRef.current = null;
  }, []);
  const handleEditFieldChange = useCallback(
    <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => {
      captureScroll();
      onFieldChange(field, value);
    },
    [captureScroll, onFieldChange],
  );
  return { scrollContainerRef, handleEditFieldChange, clearPendingScroll };
}
