// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTemplate, IconTemplateOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { useSetTemplate } from '../hooks/useVMs';
import type { VM } from '../types';

interface MakeTemplateButtonProps {
  vm: VM;
  appearance?: 'header' | 'icon-sm';
  disabled?: boolean;
}

export function MakeTemplateButton({ vm, appearance = 'icon-sm', disabled = false }: MakeTemplateButtonProps) {
  const { t } = useTranslation('vm');
  const setTemplate = useSetTemplate();
  const [isOpen, setOpen] = useState(false);

  const reverting = vm.isTemplate;
  const live = vm.state === 'running' || vm.state === 'paused' || vm.state === 'stopping';
  const blocked = disabled || setTemplate.isPending || (!reverting && live);
  const label = reverting ? t('actions.unTemplate') : t('actions.makeTemplate');
  const hint = !reverting && live ? `${label} — ${t('hardware.mustBeStopped')}` : label;
  const Icon = reverting ? IconTemplateOff : IconTemplate;

  const confirmAction = () =>
    setTemplate.mutate({ id: vm.id, isTemplate: !reverting }, { onSettled: () => setOpen(false) });

  const trigger =
    appearance === 'header' ? (
      <HeaderActionButton
        tooltip={hint}
        permission={VM_PERM.MANAGE}
        onClick={() => setOpen(true)}
        disabled={blocked}
        icon={<Icon className="size-3.5" />}
      />
    ) : (
      <Tooltip>
        <TooltipTrigger asChild>
          <span>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              permission={VM_PERM.MANAGE}
              disabled={blocked}
              onClick={() => setOpen(true)}
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>{hint}</TooltipContent>
      </Tooltip>
    );

  return (
    <>
      {trigger}
      <ConfirmDialog
        open={isOpen}
        onOpenChange={setOpen}
        variant="default"
        title={reverting ? t('template.revert', { name: vm.name }) : t('template.title', { name: vm.name })}
        description={reverting ? t('template.revertBody') : t('template.body')}
        confirmLabel={reverting ? t('actions.unTemplate') : t('template.submit')}
        loading={setTemplate.isPending}
        onConfirm={confirmAction}
      />
    </>
  );
}
