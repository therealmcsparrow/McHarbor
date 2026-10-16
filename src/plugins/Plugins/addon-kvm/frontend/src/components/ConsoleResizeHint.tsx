// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { resizeBlocker, type ConsoleDisplayMode } from '../console-resize';

/**
 * Says why the guest is not following the window, instead of silently scaling.
 *
 * A host-side blocker (adapter, QEMU) is known up front. A guest that never
 * answers is only known after the fact, and the agent it lacks is named,
 * because qemu-guest-agent being connected reads as "the agent is there".
 */
export function ConsoleResizeHint({
  kind,
  mode,
  adapter,
  hostResize,
  guestUnanswered,
}: {
  kind: 'vnc' | 'spice';
  mode: ConsoleDisplayMode;
  adapter: string | undefined;
  hostResize: boolean | undefined;
  guestUnanswered: boolean;
}) {
  const { t } = useTranslation('vm');
  if (mode !== 'resize') return null;

  const blocker = resizeBlocker(kind, adapter, hostResize);
  let message: string | null = null;
  if (blocker === 'adapter') message = t('console.display.adapter', { adapter });
  else if (blocker === 'adapterQxl') message = t('console.display.adapterQxl');
  else if (blocker === 'host') message = t('console.display.host');
  else if (guestUnanswered) message = t(kind === 'spice' ? 'console.display.guestSpice' : 'console.display.guestVnc');
  if (!message) return null;

  return (
    <output className="mb-3 flex items-start gap-2 text-xs text-warning">
      <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
      {message}
    </output>
  );
}
