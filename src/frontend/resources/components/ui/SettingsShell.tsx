// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type ReactNode } from 'react';
import { Box, Flex, Stack } from './Layout';
import { FormActions } from './FormActions';
import { Text as UiText, Title } from './Typography';
import { cn } from '@resources/utils/cn';

type SettingsShellProps = {
  /** Page title shown in the header. */
  title: ReactNode;
  /** Optional subtitle / description. */
  description?: ReactNode;
  /** Page body — typically Sections + Fields + the form chrome. */
  children: ReactNode;
  /** Sticky footer props — when provided, a `FormActions` bar renders at the
   *  bottom of the shell with save/cancel handling. */
  footer?: {
    onSave: () => void;
    onCancel?: () => void;
    canSave?: boolean;
    isSaving?: boolean;
    saveLabel?: string;
    savingLabel?: string;
    cancelLabel?: string;
    warning?: ReactNode;
    extraActions?: ReactNode;
    /** Disable the sticky positioning so the footer renders inline. */
    sticky?: boolean;
    /** Hide the cancel button. */
    hideCancel?: boolean;
  };
  /** Optional extra header actions (rendered to the right of the title). */
  headerActions?: ReactNode;
  /** Suppress the internal title block — for callers that already render a
   *  page hero and would otherwise show the same heading twice. */
  hideHeader?: boolean;
  /** Inner padding class override. Defaults to `p-6`. */
  innerClassName?: string;
  /** Outer container className. */
  className?: string;
};

/**
 * SettingsShell - a consistent wrapper for any page that renders a
 * settings-style form (full-page form, config edit panel, etc.).
 *
 * It provides:
 *   - A title + description block, with optional header actions
 *   - The form body (children)
 *   - An optional sticky FormActions footer
 *
 * Use it for Settings tabs that own their own save flow, container
 * config edit sections, and anywhere else a PageHeader + FormActions
 * pairing would otherwise be hand-rolled.
 */
export function SettingsShell({
  title,
  description,
  children,
  footer,
  headerActions,
  hideHeader = false,
  innerClassName,
  className,
}: SettingsShellProps) {
  return (
    <Box className={cn('flex h-full min-h-0 flex-col', className)}>
      {!hideHeader && (
        <Box className={cn('flex-shrink-0 px-6 pt-6 pb-4', innerClassName)}>
          <Flex align="center" justify="between" gap={3}>
            <Box className="min-w-0 flex-1">
              <Title size="sm" className="truncate leading-tight">
                {title}
              </Title>
              {description && (
                <UiText variant="muted" size="xs" className="mt-1 truncate font-light">
                  {description}
                </UiText>
              )}
            </Box>
            {headerActions && (
              <Flex align="center" gap={2} className="shrink-0">
                {headerActions}
              </Flex>
            )}
          </Flex>
        </Box>
      )}

      <Box className={cn('min-h-0 flex-1 overflow-auto px-6 pb-6', innerClassName)}>
        <Stack gap={4}>{children}</Stack>
      </Box>

      {footer && (
        <FormActions
          onSave={footer.onSave}
          onCancel={footer.onCancel}
          canSave={footer.canSave}
          isSaving={footer.isSaving}
          saveLabel={footer.saveLabel}
          savingLabel={footer.savingLabel}
          cancelLabel={footer.cancelLabel}
          warning={footer.warning}
          extraActions={footer.extraActions}
          sticky={footer.sticky ?? true}
          hideCancel={footer.hideCancel}
        />
      )}
    </Box>
  );
}
