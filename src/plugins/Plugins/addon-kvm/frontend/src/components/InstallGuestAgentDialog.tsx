// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconDisc } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { VM_PERM } from '../permissions';
import { useAttachMedia } from '../hooks/useVMs';
import { useComputeImages } from '../hooks/useComputeResources';
import type { VM } from '../types';
import { CommandLine } from './CommandLine';

type Family = 'linux' | 'windows' | 'bsd';

const COMMANDS: Record<Family, string[]> = {
  linux: [
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: shell command the operator runs in the guest
    'sudo apt install -y qemu-guest-agent',
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: shell command the operator runs in the guest
    'sudo dnf install -y qemu-guest-agent',
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: shell command the operator runs in the guest
    'sudo systemctl enable --now qemu-guest-agent',
  ],
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: shell commands the operator runs in the guest
  bsd: ['sudo pkg install -y qemu-guest-agent', 'sudo service qemu-guest-agent enable'],
  windows: ['D:\\virtio-win-guest-tools.exe'],
};

function familyOf(vm: VM): Family {
  if (vm.guestOsFamily === 'windows') return 'windows';
  if (vm.guestOsFamily === 'bsd') return 'bsd';
  return 'linux';
}

export function InstallGuestAgentDialog({ vm, onClose }: { vm: VM; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const attach = useAttachMedia();
  const { data: discs } = useComputeImages(vm.environmentId, 'iso');

  const [family, setFamily] = useState<Family>(() => familyOf(vm));
  const [pickedImageId, setImageId] = useState('');

  const candidates = (discs ?? []).filter((entry) => (family === 'windows' ? /virtio/i.test(entry.displayName) : true));

  const imageId = candidates.some((entry) => entry.id === pickedImageId) ? pickedImageId : (candidates[0]?.id ?? '');

  const disc = candidates.find((entry) => entry.id === imageId);
  const connected = vm.guestAgentState === 'connected';
  const running = vm.state === 'running';

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('guestAgent.install.title')}</DialogTitle>
          <DialogDescription>{t('guestAgent.install.body')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {connected ? (
            <div className="flex items-start gap-2 rounded-md bg-success/10 p-3">
              <IconCheck className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
              <p className="text-sm text-foreground">{t('guestAgent.install.alreadyConnected')}</p>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t('guestAgent.install.channelReady')}</p>
          )}

          <div>
            <Label htmlFor="agent-family">{t('guestAgent.install.family')}</Label>
            <Select
              value={family}
              ariaLabel={t('guestAgent.install.family')}
              options={[
                {
                  value: 'linux',
                  label: t('guestAgent.install.families.linux'),
                },
                {
                  value: 'windows',
                  label: t('guestAgent.install.families.windows'),
                },
                { value: 'bsd', label: t('guestAgent.install.families.bsd') },
              ]}
              onChange={(value) => setFamily(value as Family)}
            />
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">{t('guestAgent.install.step1')}</p>
            {candidates.length === 0 ? (
              <p className="text-sm text-warning">
                {family === 'windows' ? t('guestAgent.install.noVirtioDisc') : t('guestAgent.install.noDiscs')}
              </p>
            ) : (
              <>
                <Select
                  value={imageId}
                  ariaLabel={t('guestAgent.install.disc')}
                  options={candidates.map((entry) => ({
                    value: entry.id,
                    label: `${entry.displayName} (${entry.arch})`,
                  }))}
                  onChange={setImageId}
                />
                <Button
                  size="sm"
                  variant="outline"
                  permission={VM_PERM.MANAGE}
                  disabled={!disc?.cache?.localPath || running || attach.isPending}
                  onClick={() =>
                    disc?.cache?.localPath &&
                    attach.mutate({
                      id: vm.id,
                      imageId: disc.id,
                      sourcePath: disc.cache.localPath,
                      label: disc.displayName,
                    })
                  }
                >
                  {attach.isPending ? (
                    <Spinner className="mr-2 h-4 w-4" />
                  ) : (
                    <IconDisc className="mr-1.5 h-4 w-4" aria-hidden="true" />
                  )}
                  {t('guestAgent.install.attach')}
                </Button>
                {running && <p className="text-xs text-muted-foreground">{t('hardware.mustBeStopped')}</p>}
              </>
            )}
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">{t('guestAgent.install.step2')}</p>
            <p className="text-xs text-muted-foreground">{t(`guestAgent.install.notes.${family}`)}</p>
            {COMMANDS[family].map((command) => (
              <CommandLine key={command} command={command} />
            ))}
          </div>

          <p className="text-xs text-muted-foreground">{t('guestAgent.install.step3')}</p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.close', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
