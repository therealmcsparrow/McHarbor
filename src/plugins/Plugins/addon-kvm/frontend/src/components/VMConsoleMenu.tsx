// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconClipboard, IconDotsVertical, IconPhoto } from '@tabler/icons-react';
import RFBModule from '@novnc/novnc/lib/rfb.js';
import { Button } from '@resources/components/ui/Button';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { consoleRFB } from '../console-sessions';
import { CommandLine } from './CommandLine';

type RFB = InstanceType<typeof RFBModule>;

export function ConsoleMenu({ vmId, vmName, environmentId }: { vmId: string; vmName: string; environmentId: string }) {
  const { t } = useTranslation('vm');

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="icon-sm" variant="outline" aria-label={t('console.moreActions')} title={t('console.moreActions')}>
          <IconDotsVertical className="h-4 w-4" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 max-w-[calc(100vw-2rem)] space-y-2">
        <Button
          size="sm"
          variant="outline"
          className="w-full justify-start"
          onClick={() => void pasteToGuest(consoleRFB(vmId))}
        >
          <IconClipboard className="mr-2 h-4 w-4" aria-hidden="true" />
          {t('console.paste')}
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="w-full justify-start"
          onClick={() => downloadScreenshot(consoleRFB(vmId), vmName)}
        >
          <IconPhoto className="mr-2 h-4 w-4" aria-hidden="true" />
          {t('console.screenshot')}
        </Button>
        <div className="space-y-1.5 border-t border-border pt-2">
          <p className="text-xs font-medium text-foreground">{t('console.external.title')}</p>
          <p className="text-xs text-muted-foreground">{t('console.external.run')}</p>
          <CommandLine command={tunnelCommand(vmId, environmentId)} />
          <p className="text-xs text-muted-foreground">{t('console.external.connect', { address: TUNNEL_ADDRESS })}</p>
        </div>
      </PopoverContent>
    </Popover>
  );
}

/** Where `--console vnc` listens unless told otherwise; the CLI's own default. */
const TUNNEL_ADDRESS = 'localhost:5900';

/**
 * The CLI command that serves this console to a native VNC client. QEMU's VNC is
 * a unix socket on the host, so the CLI tunnels it through the same ticketed
 * console connection the browser uses, and nothing is opened on the host.
 */
function tunnelCommand(vmId: string, environmentId: string): string {
  return `mcharbor kvm vm console ${vmId} --console vnc --env ${environmentId}`;
}

/**
 * Paste the browser's clipboard into the guest.
 *
 * Reading the clipboard needs a user gesture and permission, and both can be
 * refused; a refusal is silent here rather than an error, because the operator
 * who declined the prompt already knows why nothing arrived.
 */
async function pasteToGuest(rfb: RFB | null) {
  if (!rfb || !navigator.clipboard?.readText) return;
  try {
    const text = await navigator.clipboard.readText();
    if (text) rfb.clipboardPasteFrom(text);
  } catch {
    // Declined or unavailable. Nothing to recover.
  }
}

/**
 * Save what is on the screen.
 *
 * The canvas is same-origin — the frames arrive over the WebSocket rather than
 * from an image host — so it is not tainted and can be read back.
 */
function downloadScreenshot(rfb: RFB | null, nameValue: string) {
  if (!rfb) return;
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- unavoidable cast: _canvas is a private noVNC field with no public accessor
  const canvas = (rfb as unknown as { _canvas?: HTMLCanvasElement })._canvas;
  if (!canvas) return;

  canvas.toBlob((blob) => {
    if (!blob) return;
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${nameValue}-${new Date().toISOString().slice(0, 19).replace(/:/g, '')}.png`;
    link.click();
    // Revoke on the next tick, after the browser has taken the blob.
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
  });
}
