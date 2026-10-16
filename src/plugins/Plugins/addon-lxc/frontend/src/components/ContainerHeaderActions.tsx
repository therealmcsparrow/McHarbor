// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconArrowLeft,
  IconCopy,
  IconPlayerPlay,
  IconPlayerStop,
  IconSnowflake,
  IconSparkles,
  IconTrash,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { CT_PERM } from '../permissions';
import { useDiagnose, useLifecycle } from '../hooks/useContainers';
import type { Container } from '../types';

interface ContainerHeaderActionsProps {
  container: Container;
  live: boolean;
  disabled: boolean;
  diagnose: ReturnType<typeof useDiagnose>;
  lifecycle: ReturnType<typeof useLifecycle>;
  onBack: () => void;
  onClone: () => void;
  onDelete: () => void;
}

export function ContainerHeaderActions({
  container,
  live,
  disabled,
  diagnose,
  lifecycle,
  onBack,
  onClone,
  onDelete,
}: ContainerHeaderActionsProps) {
  const { t } = useTranslation('ct');

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="ghost" size="icon" aria-label={t('actions.back')} onClick={onBack}>
        <IconArrowLeft className="h-4 w-4" aria-hidden="true" />
      </Button>
      {/*
        The diagnose action appears only when something is actually wrong.
        Offering "ask AI what is wrong" beside a healthy container is noise,
        and it costs tokens.
      */}
      {(container.state === 'crashed' || container.lastError) && (
        <Button
          variant="outline"
          size="sm"
          permission={CT_PERM.VIEW}
          disabled={diagnose.isPending}
          onClick={() => diagnose.mutate(container.id)}
        >
          <IconSparkles className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('actions.diagnose')}
        </Button>
      )}

      {live && (
        <Button
          variant="outline"
          size="sm"
          permission={CT_PERM.MANAGE}
          disabled={disabled}
          onClick={() =>
            lifecycle.mutate({
              id: container.id,
              action: container.state === 'frozen' ? 'unfreeze' : 'freeze',
            })
          }
        >
          <IconSnowflake className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {container.state === 'frozen' ? t('actions.unfreeze') : t('actions.freeze')}
        </Button>
      )}

      {live ? (
        <Button
          variant="outline"
          size="sm"
          permission={CT_PERM.MANAGE}
          disabled={disabled}
          onClick={() => lifecycle.mutate({ id: container.id, action: 'shutdown' })}
        >
          <IconPlayerStop className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('actions.shutdown')}
        </Button>
      ) : (
        <Button
          size="sm"
          permission={CT_PERM.MANAGE}
          disabled={disabled || container.isTemplate}
          onClick={() => lifecycle.mutate({ id: container.id, action: 'start' })}
        >
          <IconPlayerPlay className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('actions.start')}
        </Button>
      )}

      <Button variant="outline" size="sm" permission={CT_PERM.MANAGE} disabled={disabled} onClick={onClone}>
        <IconCopy className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {t('actions.clone')}
      </Button>

      <Button
        variant="ghost"
        size="icon"
        aria-label={t('actions.delete')}
        permission={CT_PERM.DELETE}
        onClick={onDelete}
      >
        <IconTrash className="h-4 w-4" aria-hidden="true" />
      </Button>
    </div>
  );
}
