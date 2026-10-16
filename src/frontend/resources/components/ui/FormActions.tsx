// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './Button';
import { cn } from '@resources/utils/cn';

type FormActionsVariant = 'default' | 'compact';

type FormActionsProps = {
  onSave: () => void;
  onCancel?: () => void;
  /** Disable the save button — typically when the form is dirty-free or saving. */
  canSave?: boolean;
  isSaving?: boolean;
  /** Save label override. Defaults to the shared `common.actions.save` key. */
  saveLabel?: string;
  /** Save-in-progress label override. Defaults to `common.actions.saving`. */
  savingLabel?: string;
  /** Cancel label override. Defaults to `common.actions.cancel`. */
  cancelLabel?: string;
  /** Sticky to the bottom of the form. Defaults to `true`. */
  sticky?: boolean;
  /** Optional warning/inline status shown on the left side (icon + text). */
  warning?: ReactNode;
  /** Optional content rendered before the save/cancel buttons on the right. */
  extraActions?: ReactNode;
  variant?: FormActionsVariant;
  className?: string;
  /** Hide the cancel button entirely. */
  hideCancel?: boolean;
  /** Save button variant override. Defaults to `default`. */
  saveVariant?: 'default' | 'destructive' | 'outline';
};

/**
 * FormActions is the canonical sticky footer for save/cancel-style form
 * submission bars. It is intentionally simple — pair it with `<Section>`
 * and `<Field>` inside a flex column to get a consistent save pattern
 * across the Settings pages and large create/edit dialogs.
 *
 * `warning` is the typical hook for "this change recreates the
 * container" / "this only applies on next save" style banners.
 */
export function FormActions({
  onSave,
  onCancel,
  canSave = true,
  isSaving = false,
  saveLabel,
  savingLabel,
  cancelLabel,
  sticky = true,
  warning,
  extraActions,
  variant = 'default',
  className,
  hideCancel = false,
  saveVariant = 'default',
}: FormActionsProps) {
  const { t } = useTranslation('common');
  return (
    <div
      className={cn(
        'flex flex-wrap items-center justify-between gap-3 border-t border-border bg-card px-5',
        variant === 'compact' ? 'py-2' : 'py-3',
        sticky && 'sticky bottom-0 z-10',
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">{warning}</div>
      <div className="flex flex-wrap items-center gap-2">
        {extraActions}
        {!hideCancel && onCancel && (
          <Button
            variant="outline"
            size={variant === 'compact' ? 'sm' : 'default'}
            onClick={onCancel}
            disabled={isSaving}
          >
            {cancelLabel ?? t('actions.cancel')}
          </Button>
        )}
        <Button
          variant={saveVariant}
          size={variant === 'compact' ? 'sm' : 'default'}
          onClick={onSave}
          disabled={!canSave || isSaving}
        >
          {isSaving ? (savingLabel ?? t('actions.saving')) : (saveLabel ?? t('actions.save'))}
        </Button>
      </div>
    </div>
  );
}
