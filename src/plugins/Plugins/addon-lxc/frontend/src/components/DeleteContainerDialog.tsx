// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
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
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { CT_PERM } from '../permissions';
import { useDeleteContainer } from '../hooks/useContainers';
import type { Container } from '../types';

/**
 * Destroy a container.
 *
 * Two separate decisions, and the default is the destructive one — the reverse
 * of addon-kvm's dialog, deliberately. A VM's disks are routinely moved to
 * another host, so keeping them is a normal thing to want. A container's rootfs
 * is a directory of unpacked distribution files that only means anything to
 * this container, so keeping it is the unusual choice and deleting is what the
 * operator almost always means.
 *
 * The name still has to be typed, because deleting a rootfs is permanent and a
 * mis-click in a table row should not be enough.
 */
export function DeleteContainerDialog({
  container,
  onClose,
  onDeleted,
}: {
  container: Container;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const { t } = useTranslation('ct');
  const remove = useDeleteContainer();

  const [keepRootfs, setKeepRootfs] = useState(false);
  const [typedName, setTypedName] = useState('');

  // Required only for the destructive path. Demanding it for the harmless one
  // trains people to type it without reading.
  const confirmed = keepRootfs || typedName.trim() === container.name;

  return (
    <Dialog open onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('delete.title', { name: container.name })}</DialogTitle>
          <DialogDescription>{t('delete.body')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Label htmlFor="ct-keep-rootfs" className="text-sm font-normal">
                {t('delete.keepRootfs')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">{t('delete.keepRootfsHint')}</p>
            </div>
            <Switch
              id="ct-keep-rootfs"
              checked={keepRootfs}
              onCheckedChange={(next) => {
                setKeepRootfs(next);
                if (next) setTypedName('');
              }}
            />
          </div>

          {!keepRootfs && (
            <div className="space-y-3 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <div className="flex items-start gap-2">
                <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
                <p className="text-sm font-medium text-foreground">{t('delete.body')}</p>
              </div>
              <div>
                <Label htmlFor="ct-confirm">{t('delete.confirm')}</Label>
                <Input
                  id="ct-confirm"
                  value={typedName}
                  onChange={(ev) => setTypedName(ev.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
            </div>
          )}

          {/*
            A template whose rootfs still backs a copy-on-write clone cannot go:
            deleting it would corrupt every clone at once, silently, at the next
            read. The backend refuses it too — this is so the operator learns it
            before they type the name rather than after.
          */}
          {container.isTemplate && (
            <p className="rounded-md bg-warning/10 p-3 text-sm text-foreground">{t('delete.templateInUse')}</p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button
            variant="destructive"
            permission={CT_PERM.DELETE}
            disabled={!confirmed || remove.isPending}
            onClick={() =>
              remove.mutate(
                { id: container.id, keepRootfs },
                {
                  onSuccess: () => {
                    onClose();
                    onDeleted();
                  },
                },
              )
            }
          >
            {remove.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('actions.delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
