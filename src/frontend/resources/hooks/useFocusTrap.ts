// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';

type FocusTrapOptions = {
  active?: boolean;
  // When the trap unmounts (or `active` flips to false), return
  // focus to the element that was focused when it mounted. Default
  // is `true` so call sites don't need to remember to do this.
  restoreFocus?: boolean;
  // Optional initial focus override. When omitted, the first
  // focusable descendant of the container receives focus.
  initialFocus?: () => HTMLElement | null;
};

// FOCUSABLE_SELECTORS — the union of element types that can
// receive keyboard focus. Order matters only for the initial
// focus hint; Tab navigation traverses the DOM naturally.
const FOCUSABLE_SELECTORS = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'iframe',
  'object',
  'embed',
  '[tabindex]:not([tabindex="-1"])',
  '[contenteditable="true"]',
  'audio[controls]',
  'video[controls]',
  'summary',
].join(',');

function getFocusableElements(container: HTMLElement): HTMLElement[] {
  const nodes = Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS));
  return nodes.filter((node) => {
    if (node.hasAttribute('disabled')) return false;
    if (node.getAttribute('aria-hidden') === 'true') return false;
    const rect = node.getBoundingClientRect();
    // Hidden via inline style or display:none.
    if (rect.width === 0 && rect.height === 0) {
      const style = window.getComputedStyle(node);
      if (style.visibility === 'hidden' || style.display === 'none') return false;
    }
    return true;
  });
}

function getInitialFocus(container: HTMLElement, override?: () => HTMLElement | null): HTMLElement | null {
  if (override) {
    const explicit = override();
    if (explicit) return explicit;
  }
  const focusables = getFocusableElements(container);
  return focusables[0] ?? null;
}

// useFocusTrap traps keyboard focus inside the returned ref's
// container while `active` is true. Use it for modals, menus,
// drawers, and any transient overlay where the user should not
// be able to Tab into the page behind.
//
// Note: Radix `<Dialog>` already traps focus internally, so this
// hook is intended for ad-hoc traps (popovers, command palettes,
// side panels) where the underlying primitive does not provide
// its own trap.
export function useFocusTrap<T extends HTMLElement>(options: FocusTrapOptions = {}): React.RefObject<T | null> {
  const { active = true, restoreFocus = true, initialFocus } = options;
  const containerRef = useRef<T | null>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!active) return;
    const container = containerRef.current;
    if (!container) return;
    const trapEl: HTMLElement = container;

    // Capture the element that was focused before the trap so we
    // can restore it on unmount.
    previouslyFocusedRef.current = document.activeElement as HTMLElement | null;

    const focusTarget = getInitialFocus(container, initialFocus);
    if (focusTarget) {
      focusTarget.focus();
    } else {
      // Fall back to focusing the container itself so screen
      // readers announce the boundary.
      container.tabIndex = -1;
      container.focus();
    }

    function handleKeyDown(evt: KeyboardEvent) {
      if (evt.key !== 'Tab') return;
      const focusables = getFocusableElements(trapEl);
      if (focusables.length === 0) {
        evt.preventDefault();
        return;
      }
      const first = focusables[0]!;
      const last = focusables[focusables.length - 1]!;
      const activeEl = document.activeElement as HTMLElement | null;

      if (evt.shiftKey) {
        if (activeEl === first || !trapEl.contains(activeEl)) {
          evt.preventDefault();
          last.focus();
        }
      } else if (activeEl === last) {
        evt.preventDefault();
        first.focus();
      }
    }

    trapEl.addEventListener('keydown', handleKeyDown);

    return () => {
      trapEl.removeEventListener('keydown', handleKeyDown);
      if (restoreFocus && previouslyFocusedRef.current) {
        const previous = previouslyFocusedRef.current;
        // Only restore if the element is still connected to the
        // document. Some apps remove the trigger on mount.
        if (previous.isConnected) {
          previous.focus();
        }
      }
    };
  }, [active, restoreFocus, initialFocus]);

  return containerRef;
}
