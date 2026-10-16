// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff, IconLock } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { PERM } from '@core/auth/permissionKeys';
import { ENV_REDACTION_MARKER, isRedactedValue } from '@resources/utils/env-redaction';

type EnvironmentVarRowProps = {
  envKey: string;
  maskedValue: string;
  revealedValue?: string;
  visible: boolean;
  onToggleVisibility: (key: string) => void;
  onPromote: (key: string, value: string) => void;
};

export function EnvironmentVarRow({
  envKey,
  maskedValue,
  revealedValue,
  visible,
  onToggleVisibility,
  onPromote,
}: EnvironmentVarRowProps) {
  const { t } = useTranslation('stacks');
  const { t: ts } = useTranslation('secrets');
  const promoteLabel = ts('actions.promote');

  const promoteDisabled = !visible || revealedValue === undefined || revealedValue.startsWith('${secret:');
  // A masked row can only be shown through the audited reveal, which needs stacks.manage;
  // an unmasked row is already in the payload and needs nothing.
  const masked = isRedactedValue(maskedValue);

  return (
    <tr className="border-b border-border last:border-0 hover:bg-muted/30">
      <td className="px-4 py-2.5 font-mono text-xs">{envKey}</td>
      <td className="px-4 py-2.5 font-mono text-xs text-muted-foreground">
        {visible ? (revealedValue ?? maskedValue) : ENV_REDACTION_MARKER}
      </td>
      <td className="px-2 py-2.5">
        <span className="sr-only">{t('columns.actions')}</span>
        <div className="flex items-center justify-end gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  permission={masked ? PERM.STACKS_MANAGE : undefined}
                  onClick={() => onToggleVisibility(envKey)}
                  aria-label={visible ? t('detail.hideValue') : t('detail.showValue')}
                >
                  {visible ? (
                    <IconEyeOff className="h-3.5 w-3.5 text-muted-foreground" />
                  ) : (
                    <IconEye className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{visible ? t('detail.hideValue') : t('detail.showValue')}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  permission={PERM.SECRETS_MANAGE}
                  variant="ghost"
                  size="icon-sm"
                  aria-label={promoteLabel}
                  disabled={promoteDisabled}
                  onClick={() => onPromote(envKey, revealedValue ?? '')}
                >
                  <IconLock className="h-3.5 w-3.5 text-muted-foreground" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{promoteLabel}</TooltipContent>
          </Tooltip>
        </div>
      </td>
    </tr>
  );
}
