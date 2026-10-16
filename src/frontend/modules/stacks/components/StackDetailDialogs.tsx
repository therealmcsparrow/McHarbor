// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { LinkContainerDialog } from '@resources/components/LinkContainerDialog';
import { useDeleteStack } from '@resources/hooks/useStacks';
import type { StackDetail } from '@resources/hooks/useStacks';
import { TakeOverDialog } from './TakeOverDialog';
import { StackDownDialog } from './StackDownDialog';

export function useStackDialogState() {
  const [removeOpen, setRemoveOpen] = useState(false);
  const [downOpen, setDownOpen] = useState(false);
  const [takeOverOpen, setTakeOverOpen] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  return {
    removeOpen,
    setRemoveOpen,
    downOpen,
    setDownOpen,
    takeOverOpen,
    setTakeOverOpen,
    linkOpen,
    setLinkOpen,
  };
}

type StackDetailDialogsProps = {
  stack: StackDetail;
  locked: boolean;
  dialogs: ReturnType<typeof useStackDialogState>;
};

export function StackDetailDialogs({ stack, locked, dialogs }: StackDetailDialogsProps) {
  const { t } = useTranslation('stacks');
  const navigate = useNavigate();
  const deleteStack = useDeleteStack();
  const { removeOpen, setRemoveOpen, downOpen, setDownOpen, takeOverOpen, setTakeOverOpen, linkOpen, setLinkOpen } =
    dialogs;

  return (
    <>
      <TakeOverDialog open={takeOverOpen} onOpenChange={setTakeOverOpen} stackName={stack.name} />

      <StackDownDialog
        stack={downOpen ? stack : null}
        blocked={locked}
        onOpenChange={(next) => !next && setDownOpen(false)}
      />

      <LinkContainerDialog open={linkOpen} onOpenChange={setLinkOpen} initialStackName={stack.name} fixedStack />

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={(next) => !next && setRemoveOpen(false)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription', { name: stack.name })}
        confirmLabel={t('confirm.removeLabel')}
        onConfirm={() => {
          if (locked) {
            setRemoveOpen(false);
            return;
          }
          deleteStack.mutate(stack.name, {
            onSuccess: () => navigate('/stacks'),
          });
          setRemoveOpen(false);
        }}
        loading={deleteStack.isPending}
      />
    </>
  );
}
