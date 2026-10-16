// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { InsertSecretDialog } from './InsertSecretDialog';
import { asInsertTarget, insertSecretReference, type InsertTarget } from './insert-target';

/**
 * Shift + right-click a text or number field anywhere in McHarbor and pick a
 * stored secret; the field receives `${secret:NAME}`, which the server resolves
 * at deploy time.
 *
 * Mounted once, in AppLayout. One listener on the document rather than a prop
 * threaded through every form: the fields that want this are in the stack
 * editor, the App Store install form, workflow nodes, registry and git
 * credentials, SMTP settings and half a dozen plugin panels, and wiring each
 * one individually would guarantee the next form forgets.
 *
 * Three things it deliberately does **not** do:
 *
 *  1. **It never takes the native menu away.** Shift is the opt-in; a plain
 *     right-click, or one with Ctrl, Alt or Meta, passes straight through to
 *     the browser, so copy, paste and spellcheck are never displaced.
 *  2. **It does nothing at all without `secrets.view`.** A user who cannot list
 *     secrets keeps the ordinary context menu everywhere, with no behaviour to
 *     explain and nothing to be puzzled by.
 *  3. **It never handles a value.** The dialog lists names; the field receives
 *     a reference. Reading an actual value is the reveal endpoint's job and
 *     costs an elevation, a step-up and an audit entry.
 */
export function SecretInsertProvider() {
  const canViewSecrets = usePermission(PERM.SECRETS_VIEW);
  const [isOpen, setOpen] = useState(false);

  // The field the menu was opened on, held across the dialog's lifetime.
  // A ref rather than state: it is not rendered, and re-rendering the
  // provider on every right-click would remount the dialog.
  const targetRef = useRef<InsertTarget | null>(null);

  useEffect(() => {
    if (!canViewSecrets) return;

    const onContextMenu = (evt: MouseEvent) => {
      // Shift is the opt-in; a plain right-click stays the browser's own menu.
      if (!evt.shiftKey || evt.ctrlKey || evt.altKey || evt.metaKey) return;

      const target = asInsertTarget(evt.target);
      if (!target) return;

      evt.preventDefault();
      targetRef.current = target;
      setOpen(true);
    };

    document.addEventListener('contextmenu', onContextMenu);
    return () => document.removeEventListener('contextmenu', onContextMenu);
  }, [canViewSecrets]);

  const handleSelect = useCallback((reference: string) => {
    const target = targetRef.current;
    if (!target) return;
    // The field may have been unmounted while the dialog was open — a tab
    // switch, a closing drawer. Writing to a detached node is harmless but
    // pointless, and focusing one throws in some browsers.
    if (!target.isConnected) return;
    insertSecretReference(target, reference);
  }, []);

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next);
    if (!next) targetRef.current = null;
  }, []);

  if (!canViewSecrets) return null;

  return <InsertSecretDialog open={isOpen} onOpenChange={handleOpenChange} onSelect={handleSelect} />;
}
