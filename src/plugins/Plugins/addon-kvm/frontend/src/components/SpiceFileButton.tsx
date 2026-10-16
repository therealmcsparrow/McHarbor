// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconExternalLink } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { VM_PERM } from '../permissions';
import { useSpiceFile } from '../hooks/useSpiceFile';
import type { VM } from '../types';

const REMOTE_VIEWER_DOWNLOAD = 'https://virt-manager.org/download';

/**
 * A remote-viewer file for operators who prefer the native SPICE client.
 *
 * The address is optional: the agent reports its own, which is wrong when the
 * operator reaches the host by a name the host does not know it has.
 */
export function SpiceFileButton({ vm, supported, reason }: { vm: VM; supported: boolean; reason: string }) {
  const { t } = useTranslation('vm');
  const download = useSpiceFile();
  const [host, setHost] = useState('');
  const [isOpen, setOpen] = useState(false);

  const trigger = (
    <Button size="sm" variant="outline" permission={VM_PERM.CONSOLE} disabled={!supported}>
      <IconDownload className="mr-2 h-4 w-4" aria-hidden="true" />
      {t('console.spiceFile.button')}
    </Button>
  );

  if (!supported) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <TooltipFocusTarget>{trigger}</TooltipFocusTarget>
        </TooltipTrigger>
        <TooltipContent className="max-w-[320px]">{reason}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Popover open={isOpen} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent className="w-80 space-y-3">
        <p className="text-xs text-muted-foreground">{t('console.spiceFile.hint')}</p>
        <a
          href={REMOTE_VIEWER_DOWNLOAD}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-xs text-primary underline-offset-2 hover:underline"
        >
          <IconExternalLink className="h-3 w-3" aria-hidden="true" />
          {t('console.spiceFile.getClient')}
        </a>
        <div className="space-y-1">
          <Label htmlFor={`vm-vv-host-${vm.id}`}>{t('console.spiceFile.host')}</Label>
          <Input
            id={`vm-vv-host-${vm.id}`}
            value={host}
            placeholder={t('console.spiceFile.hostPlaceholder')}
            onChange={(evt) => setHost(evt.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <Button
          size="sm"
          className="w-full"
          permission={VM_PERM.CONSOLE}
          disabled={download.isPending}
          onClick={() => download.mutate({ id: vm.id, name: vm.name, host }, { onSuccess: () => setOpen(false) })}
        >
          {download.isPending && <Spinner className="mr-2 h-4 w-4" />}
          {t('console.spiceFile.download')}
        </Button>
      </PopoverContent>
    </Popover>
  );
}
