// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconX, IconPointFilled } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { evaluatePassword, type PasswordPolicy } from '@core/auth/passwordPolicy';

type RuleState = 'idle' | 'met' | 'unmet';

function ruleState(touched: boolean, met: boolean): RuleState {
  if (!touched) return 'idle';
  return met ? 'met' : 'unmet';
}

type PasswordRequirementsProps = {
  /** The candidate being typed. */
  password: string;
  /** The runtime policy, from `usePasswordPolicy()`. */
  policy: PasswordPolicy;
  /**
   * Whether the field has been touched. Before the first keystroke
   * every rule renders neutral — an untouched form covered in red
   * crosses reads as "you already did something wrong".
   */
  touched?: boolean;
  /**
   * Whether to state the no-reuse rule. False on screens that create
   * an account — a brand-new user has no password history, so the
   * line would name a rule that cannot fire.
   */
  showHistory?: boolean;
  /** Set when a field points here via `aria-describedby`. */
  id?: string;
  className?: string;
};

/**
 * Live checklist of the active password policy.
 *
 * Rendered next to every field that sets a password, so the rules are
 * visible before the user types and each one resolves as they do. The
 * server re-validates identically — this component is UX, not
 * enforcement.
 */
export function PasswordRequirements({
  password,
  policy,
  touched = password.length > 0,
  showHistory = true,
  id,
  className,
}: PasswordRequirementsProps) {
  const { t } = useTranslation('common');
  const { rules } = evaluatePassword(password, policy);

  if (rules.length === 0) return null;

  return (
    <ul
      id={id}
      className={cn('mt-1 space-y-1', className)}
      // Rules resolve as the user types; announce politely so a
      // screen reader reports progress without interrupting typing.
      aria-live="polite"
      data-testid="password-requirements"
    >
      {rules.map((rule) => {
        const state = ruleState(touched, rule.met);
        const Icon = state === 'met' ? IconCheck : state === 'unmet' ? IconX : IconPointFilled;
        return (
          <li
            key={rule.id}
            data-state={state}
            className={cn(
              'flex items-center gap-1.5 text-xs',
              state === 'met' && 'text-success',
              state === 'unmet' && 'text-destructive',
              state === 'idle' && 'text-muted-foreground',
            )}
          >
            <Icon className="size-3.5 shrink-0" aria-hidden="true" />
            {/* `min` rather than `count`: `count` is i18next's plural
                selector and would demand _one/_other key variants. */}
            <span>{t(`password.requirements.${rule.id}`, { min: rule.count })}</span>
          </li>
        );
      })}
      {showHistory && policy.historySize > 0 && (
        <li className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <IconPointFilled className="size-3.5 shrink-0" aria-hidden="true" />
          <span>{t('password.requirements.noReuse', { history: policy.historySize })}</span>
        </li>
      )}
    </ul>
  );
}
