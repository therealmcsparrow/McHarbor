// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, type ReactNode } from 'react';
import type { UseMutationResult } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { isApprovalRequired } from '@resources/utils/api-mutation';
import { ConfirmDialog } from './ConfirmDialog';

type MutationConfirmDialogProps<TData, TVariables> = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  confirmLabel?: string;
  variant?: 'destructive' | 'default' | 'caution';
  permission?: string | string[];
  mutation: UseMutationResult<TData, Error, TVariables>;
  variables: TVariables;
  blocked?: boolean;
  disabled?: boolean;
  onSuccess?: (data: TData) => void;
  children?: ReactNode;
};

export function MutationConfirmDialog<TData, TVariables>({
  open: isOpen,
  onOpenChange,
  title,
  description,
  confirmLabel,
  variant = 'destructive',
  permission,
  mutation,
  variables,
  blocked = false,
  disabled = false,
  onSuccess,
  children,
}: MutationConfirmDialogProps<TData, TVariables>) {
  const { t } = useTranslation('common');
  const { reset } = mutation;
  const held = mutation.isError && isApprovalRequired(mutation.error);

  useEffect(() => {
    if (isOpen) reset();
  }, [isOpen, reset]);

  const handleConfirm = () => {
    if (blocked) {
      onOpenChange(false);
      return;
    }
    mutation.mutate(variables, {
      onSuccess: (data) => {
        onOpenChange(false);
        onSuccess?.(data);
      },
    });
  };

  return (
    <ConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      confirmLabel={confirmLabel}
      variant={variant}
      loading={mutation.isPending}
      disabled={held || disabled}
      permission={permission}
      onConfirm={handleConfirm}
    >
      {children}
      {held && (
        <output className="block rounded-md border border-border bg-muted p-3 text-xs text-foreground">
          <p className="font-medium">{t('approvalHeld.title')}</p>
          <p className="mt-1 break-words whitespace-pre-wrap text-muted-foreground">{mutation.error.message}</p>
        </output>
      )}
      {mutation.isError && !held && (
        <p
          role="alert"
          className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
        >
          {mutation.error.message}
        </p>
      )}
    </ConfirmDialog>
  );
}
