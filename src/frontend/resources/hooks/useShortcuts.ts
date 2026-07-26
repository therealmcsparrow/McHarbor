// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useCallback } from 'react';
import { useNavigate, useLocation } from 'react-router';

export type ShortcutMap = {
  [key: string]: () => void;
};

export type ShortcutDefinition = {
  key: string;
  description: string;
  scope?: 'global' | 'input';
  group: 'navigation' | 'actions' | 'view';
};

const DEFAULT_SHORTCUTS: ShortcutDefinition[] = [
  { key: 'mod+k', description: 'Open command palette', group: 'navigation' },
  { key: 'mod+/', description: 'Open global search', group: 'navigation' },
  { key: 'g d', description: 'Go to Dashboard', group: 'navigation' },
  { key: 'g c', description: 'Go to Containers', group: 'navigation' },
  { key: 'g i', description: 'Go to Images', group: 'navigation' },
  { key: 'g v', description: 'Go to Volumes', group: 'navigation' },
  { key: 'g n', description: 'Go to Networks', group: 'navigation' },
  { key: 'g s', description: 'Go to Stacks', group: 'navigation' },
  { key: 'g e', description: 'Go to Environments', group: 'navigation' },
  { key: 'g a', description: 'Go to App Store', group: 'navigation' },
  { key: 'g w', description: 'Go to Workflows', group: 'navigation' },
  { key: 'g g', description: 'Go to GitOps', group: 'navigation' },
  { key: 'g t', description: 'Go to Git Repositories', group: 'navigation' },
  { key: 'g l', description: 'Go to Logs', group: 'navigation' },
  { key: 'g o', description: 'Go to Containers', group: 'navigation' },
  { key: 'g r', description: 'Refresh current page', group: 'actions' },
  { key: '?', description: 'Show keyboard shortcuts', group: 'view' },
  { key: 'esc', description: 'Close dialog / panel', group: 'view', scope: 'input' },
];

export function getDefaultShortcuts(): ShortcutDefinition[] {
  return DEFAULT_SHORTCUTS;
}

function normalizeEvent(e: KeyboardEvent): string {
  const parts: string[] = [];
  if (e.metaKey || e.ctrlKey) parts.push('mod');
  if (e.shiftKey) parts.push('shift');
  if (e.altKey) parts.push('alt');
  const key = e.key.toLowerCase();
  if (key === ' ') parts.push('space');
  else if (key === 'escape') parts.push('esc');
  else if (key === '/') parts.push('/');
  else if (key === '?') parts.push('?');
  else if (key.length === 1) parts.push(key);
  else parts.push(key);
  return parts.join('+');
}

function isInEditableElement(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (target.isContentEditable) return true;
  return false;
}

export function useShortcuts(handlers: ShortcutMap): void {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const isEditable = isInEditableElement(e.target);
      const normalized = normalizeEvent(e);

      if (handlers[normalized]) {
        if (isEditable && !normalized.includes('esc') && !normalized.includes('mod+k') && !normalized.includes('mod+/')) {
          return;
        }
        e.preventDefault();
        handlers[normalized]();
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handlers]);
}

const ROUTES: Record<string, string> = {
  'g d': '/dashboard',
  'g c': '/containers',
  'g o': '/containers',
  'g i': '/images',
  'g v': '/volumes',
  'g n': '/networks',
  'g s': '/stacks',
  'g e': '/environments',
  'g a': '/appstore',
  'g w': '/workflows',
  'g g': '/gitops',
  'g t': '/git',
  'g l': '/logs',
};

export function useGlobalShortcuts(options?: {
  onRefresh?: () => void;
  onShowShortcuts?: () => void;
}): void {
  const navigate = useNavigate();
  const location = useLocation();

  const handleShortcut = useCallback(
    (key: string) => {
      if (key === 'g r' && options?.onRefresh) {
        options.onRefresh();
        return;
      }
      if (key === '?' && options?.onShowShortcuts) {
        options.onShowShortcuts();
        return;
      }
      const route = ROUTES[key];
      if (route && route !== location.pathname) {
        navigate(route);
      }
    },
    [navigate, location.pathname, options],
  );

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (isInEditableElement(e.target) && !e.key.startsWith('g')) return;
      const normalized = normalizeEvent(e);
      if (ROUTES[normalized] || normalized === 'g r' || normalized === '?') {
        if (normalized.startsWith('g') || normalized === '?') {
          e.preventDefault();
          handleShortcut(normalized);
        }
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handleShortcut]);
}
