// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { IconLink, IconUnlink } from '@tabler/icons-react';
import { api } from '@core/api/client';
import type { ContainerInfo } from '@core/types/docker';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { useContainers } from '@resources/hooks/useContainers';
import { useLinkContainerToStack, useUnlinkContainerFromStack } from '@resources/hooks/useStackLinks';
import { useEnvironmentStore } from '@resources/stores/environment';

type StackOption = {
  name: string;
  type: 'managed' | 'discovered';
};

type LinkContainerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialContainerId?: string;
  initialStackName?: string;
  fixedContainer?: boolean;
  fixedStack?: boolean;
};

function containerLabel(container: ContainerInfo) {
  const name = container.Names?.[0]?.replace(/^\//, '') ?? container.Id.slice(0, 12);
  return `${name} (${container.Image})`;
}

export function LinkContainerDialog({
  open,
  onOpenChange,
  initialContainerId = '',
  initialStackName = '',
  fixedContainer = false,
  fixedStack = false,
}: LinkContainerDialogProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const envId = useEnvironmentStore((state) => state.currentId);
  const { data: containers = [] } = useContainers(true);
  const { data: stacks = [] } = useQuery({
    queryKey: ['stacks', envId, 'link-dialog'],
    queryFn: () =>
      api
        .get<StackOption[]>('/stacks', envId ? { env: envId } : {})
        .then((response) => response.data ?? []),
    enabled: open,
  });
  const link = useLinkContainerToStack();
  const unlink = useUnlinkContainerFromStack();
  const [containerId, setContainerId] = useState(initialContainerId);
  const [stackName, setStackName] = useState(initialStackName);
  const [serviceName, setServiceName] = useState('');

  useEffect(() => {
    if (!open) return;
    setContainerId(initialContainerId);
    setStackName(initialStackName);
    setServiceName('');
  }, [open, initialContainerId, initialStackName]);

  const containerOptions = useMemo(
    () => containers.map((container) => ({ value: container.Id, label: containerLabel(container) })),
    [containers],
  );

  const stackOptions = useMemo(
    () => stacks.map((stack) => ({ value: stack.name, label: `${stack.name} (${t(`badges.${stack.type}`)})` })),
    [stacks, t],
  );

  const isPending = link.isPending || unlink.isPending;
  const canSubmit = !!containerId && !!stackName && !isPending;

  function handleSubmit() {
    if (!canSubmit) return;
    link.mutate(
      {
        containerId,
        stackName,
        serviceName: serviceName.trim() || undefined,
      },
      {
        onSuccess: () => onOpenChange(false),
      },
    );
  }

  function handleUnlink() {
    if (!containerId || isPending) return;
    unlink.mutate(containerId, {
      onSuccess: () => onOpenChange(false),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('link.title')}</DialogTitle>
          <DialogDescription>{t('link.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label className="mb-2">{t('link.container')}</Label>
            <Select
              value={containerId}
              onChange={setContainerId}
              options={containerOptions}
              disabled={fixedContainer}
            />
          </div>
          <div>
            <Label className="mb-2">{t('link.stack')}</Label>
            <Select
              value={stackName}
              onChange={setStackName}
              options={stackOptions}
              disabled={fixedStack}
            />
          </div>
          <div>
            <Label className="mb-2">{t('link.serviceName')}</Label>
            <Input
              value={serviceName}
              onChange={(event) => setServiceName(event.target.value)}
              placeholder={t('link.serviceNamePlaceholder')}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          {fixedContainer && (
            <Button variant="outline" onClick={handleUnlink} disabled={!containerId || isPending}>
              <IconUnlink className="h-4 w-4" />
              {t('link.unlink')}
            </Button>
          )}
          <Button onClick={handleSubmit} disabled={!canSubmit}>
            <IconLink className="h-4 w-4" />
            {isPending ? tc('actions.processing') : t('link.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
