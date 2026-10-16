// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Text as UiText, Heading } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Card } from '@resources/components/ui/Card';
import { usePermission } from './usePermission';

type RequirePermissionProps = {
  /** Permission key, or list of keys. When an array is supplied, ANY match grants access. */
  permission: string | string[];
  /** Fallback shown when the caller lacks the permission. Defaults to <AccessDeniedEmpty />. */
  fallback?: ReactNode;
  children: ReactNode;
};

/**
 * RequirePermission gates its children on a single permission (or any
 * of a list). Use it to hide whole UI sections from users who lack
 * the underlying capability.
 *
 * Server-side enforcement still happens on every backend endpoint;
 * this component is purely a UX convenience that prevents users from
 * seeing buttons that would 403 on click.
 *
 * Passing an empty array ALWAYS denies (there is no permission set
 * that means "any user"). Pass `["*"]` to opt out of the gate.
 */
export function RequirePermission({ permission, fallback, children }: RequirePermissionProps) {
  const allowed = usePermission(permission);
  if (allowed) return <>{children}</>;
  return <>{fallback ?? <AccessDeniedEmpty />}</>;
}

type RestrictButtonProps = {
  permission: string | string[];
  /** Element to render when the caller lacks the permission. */
  fallback?: ReactNode;
  /** Element to render when the caller has the permission. */
  children: ReactNode;
};

/**
 * RestrictButton hides a single interactive affordance (a button, a
 * menu item, a link) when the caller lacks the named permission. When
 * no fallback is provided the wrapper renders nothing — the button
 * is simply invisible.
 *
 * Use RestrictButton for "I can perform this action right now"
 * decisions. Use RequirePermission for whole sections of UI.
 */
export function RestrictButton({ permission, fallback = null, children }: RestrictButtonProps) {
  const allowed = usePermission(permission);
  return <>{allowed ? children : fallback}</>;
}

type AccessDeniedEmptyProps = {
  /** Optional override for the message shown in the empty state. */
  message?: string;
  /** Optional CTA. Useful when denying should redirect the user elsewhere. */
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
};

/**
 * AccessDeniedEmpty is the default render when RequirePermission
 * hides content. Shown as a compact "permission required" tile
 * inside the slot that would otherwise contain the gated content.
 */
export function AccessDeniedEmpty({ message, actionLabel, onAction, className }: AccessDeniedEmptyProps) {
  const { t } = useTranslation('common');
  return (
    <Card className={className}>
      <Flex direction="col" align="center" justify="center" gap={3} className="py-8 text-center">
        <Flex align="center" justify="center" aria-hidden="true" className="size-12 bg-muted text-muted-foreground">
          <IconLock className="size-5" />
        </Flex>
        <Box className="max-w-sm space-y-1">
          <Heading as="h3" size="sm" weight="semibold">
            {message ?? t('accessDenied.title')}
          </Heading>
          <UiText size="sm" tone="muted">
            {t('accessDenied.description')}
          </UiText>
        </Box>
        {actionLabel && onAction && (
          <Button variant="outline" size="sm" onClick={onAction}>
            {actionLabel}
          </Button>
        )}
      </Flex>
    </Card>
  );
}
