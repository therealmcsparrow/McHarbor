// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileUpload } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { COMPUTE_PERM } from '../permissions';
import { useImageLibrary, useUploadImage } from '../hooks/useCompute';
import { formatBytes } from '../format';
import { LibraryPanel } from './LibraryPanel';

const ACCEPTED = '.qcow2,.img,.iso,.raw,.vmdk,.vhd,.vhdx,.gz,.xz,.zst,.tar,.bz2';

interface UploadImageDialogProps {
  onClose: () => void;
}

export function UploadImageDialog({ onClose }: UploadImageDialogProps) {
  const { t } = useTranslation('compute');

  const [file, setFile] = useState<File | null>(null);
  const [kind, setKind] = useState('cloud_image');
  const [displayName, setDisplayName] = useState('');
  const [distro, setDistro] = useState('');
  const [release, setRelease] = useState('');
  const [arch, setArch] = useState('x86_64');
  const [initSystem, setInitSystem] = useState('');
  const [percent, setPercent] = useState(0);

  const { data: library } = useImageLibrary();
  const upload = useUploadImage();

  const canSubmit = Boolean(file && displayName.trim() && distro.trim()) && !upload.isPending;

  const pick = (chosen: File | null) => {
    setFile(chosen);
    if (chosen && !displayName.trim()) {
      setDisplayName(chosen.name.replace(/\.[^.]+$/, ''));
    }
  };

  const submit = () => {
    if (!file || !canSubmit) return;
    upload.mutate(
      {
        file,
        kind,
        displayName: displayName.trim(),
        distro: distro.trim(),
        release: release.trim(),
        arch,
        initSystem: initSystem || undefined,
        onProgress: setPercent,
      },
      { onSuccess: onClose },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && !upload.isPending && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('images.upload.title')}</DialogTitle>
          <DialogDescription>{t('images.upload.body')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <LibraryPanel library={library} />

          <div>
            <Label htmlFor="upload-file">{t('images.upload.file')}</Label>
            <Input
              id="upload-file"
              type="file"
              accept={ACCEPTED}
              onChange={(ev) => pick(ev.target.files?.[0] ?? null)}
            />
            <p className="mt-1 text-xs text-muted-foreground">
              {file ? t('images.upload.selected', { size: formatBytes(file.size) }) : t('images.upload.fileHint')}
            </p>
          </div>

          <div>
            <Label>{t('images.upload.kind')}</Label>
            <Select
              value={kind}
              ariaLabel={t('images.upload.kind')}
              options={[
                { value: 'cloud_image', label: t('images.kind.cloud_image') },
                { value: 'rootfs', label: t('images.kind.rootfs') },
                { value: 'iso', label: t('images.kind.iso') },
              ]}
              onChange={setKind}
            />
          </div>

          <div>
            <Label htmlFor="upload-name">{t('images.upload.displayName')}</Label>
            <Input
              id="upload-name"
              value={displayName}
              onChange={(ev) => setDisplayName(ev.target.value)}
              autoComplete="off"
            />
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <div>
              <Label htmlFor="upload-distro">{t('images.upload.distro')}</Label>
              <Input
                id="upload-distro"
                value={distro}
                onChange={(ev) => setDistro(ev.target.value)}
                autoComplete="off"
              />
            </div>
            <div>
              <Label htmlFor="upload-release">{t('images.upload.release')}</Label>
              <Input
                id="upload-release"
                value={release}
                onChange={(ev) => setRelease(ev.target.value)}
                autoComplete="off"
              />
            </div>
            <div>
              <Label>{t('images.upload.arch')}</Label>
              <Select
                value={arch}
                ariaLabel={t('images.upload.arch')}
                options={[
                  { value: 'x86_64', label: 'x86_64' },
                  // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: CPU architecture identifier
                  { value: 'aarch64', label: 'aarch64' },
                ]}
                onChange={setArch}
              />
            </div>
          </div>

          {kind === 'rootfs' && (
            <div>
              <Label>{t('images.upload.initSystem')}</Label>
              <Select
                value={initSystem}
                ariaLabel={t('images.upload.initSystem')}
                placeholder={t('images.upload.initSystemPlaceholder')}
                options={[
                  // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: init system product name
                  { value: 'systemd', label: 'systemd' },
                  // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: init system product name
                  { value: 'openrc', label: 'OpenRC' },
                ]}
                onChange={setInitSystem}
              />
              <p className="mt-1 text-xs text-muted-foreground">{t('images.upload.initSystemHint')}</p>
            </div>
          )}

          {upload.isPending && (
            <div>
              <ProgressBar size="sm" value={percent} />
              <p className="mt-1 text-xs text-muted-foreground">{t('images.upload.progress', { percent })}</p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={upload.isPending}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={COMPUTE_PERM.IMAGES_UPLOAD} disabled={!canSubmit} onClick={submit}>
            <IconFileUpload className="mr-1.5 h-4 w-4" aria-hidden="true" />
            {t('images.upload.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
