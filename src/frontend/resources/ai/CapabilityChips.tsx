// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import * as TablerIcons from '@tabler/icons-react';
import { IconAlertTriangle, IconBolt, IconCircleCheck, IconLoader2, IconLock } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import type { AICapability } from './registry';

type IconComponent = React.ComponentType<{ className?: string }>;

let tablerIconsByName: Map<string, unknown> | undefined;

function isIconComponent(value: unknown): value is IconComponent {
  return typeof value === 'function' || (typeof value === 'object' && value !== null && '$$typeof' in value);
}

// The icon name is data from the capability descriptor, and the namespace import has no index signature.
function capabilityIcon(iconName: string | undefined): IconComponent {
  tablerIconsByName ??= new Map(Object.entries(TablerIcons));
  const found = iconName ? tablerIconsByName.get(iconName) : undefined;
  return isIconComponent(found) ? found : IconBolt;
}

type Props = {
  capabilities: AICapability[];
  onInvoke: (capability: AICapability) => void;
  disabled?: boolean;
};

/**
 * CapabilityChips renders the action chips below the AI chat. Every
 * page / addon can publish capabilities via the central registry; the
 * chat dialog surfaces them here as one-click buttons.
 *
 * Destructive capabilities (danger: 'high' or variant: 'destructive')
 * trigger a confirmation dialog before invoking the handler.
 */
export function CapabilityChips({ capabilities, onInvoke, disabled }: Props) {
  const { t } = useTranslation('ai');
  const [pending, setPending] = useState<string | null>(null);
  const [confirmCap, setConfirmCap] = useState<AICapability | null>(null);
  const [result, setResult] = useState<{ id: string; ok: boolean; message: string } | null>(null);

  if (capabilities.length === 0) return null;

  async function invoke(cap: AICapability) {
    setPending(cap.id);
    setResult(null);
    try {
      await cap.run();
      setResult({ id: cap.id, ok: true, message: t('capability.done') });
      onInvoke(cap);
    } catch (err) {
      setResult({
        id: cap.id,
        ok: false,
        message: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setPending(null);
    }
  }

  function handleClick(cap: AICapability) {
    if (cap.danger === 'high' || cap.variant === 'destructive') {
      setConfirmCap(cap);
    } else {
      void invoke(cap);
    }
  }

  return (
    <>
      <div className="flex flex-wrap gap-2">
        {capabilities.map((cap) => {
          const IconComp = capabilityIcon(cap.icon);
          const isPending = pending === cap.id;
          const isDone = result?.id === cap.id;
          return (
            <Button
              key={cap.id}
              size="sm"
              variant={cap.variant ?? 'outline'}
              disabled={disabled || pending !== null}
              onClick={() => handleClick(cap)}
              title={cap.description}
              className={cn(
                'gap-1.5',
                isDone && cap.id === result?.id && !result.ok && 'border-danger/50 text-danger',
                isDone && cap.id === result?.id && result.ok && 'border-success/50 text-success',
              )}
            >
              {isPending ? (
                <IconLoader2 className="size-3.5 animate-spin" />
              ) : isDone && cap.id === result?.id && result.ok ? (
                <IconCircleCheck className="size-3.5" />
              ) : (
                <IconComp className="size-3.5" />
              )}
              {cap.label}
            </Button>
          );
        })}
      </div>

      {result && result.id === pending ? null : null}

      {confirmCap && (
        <Dialog open={!!confirmCap} onOpenChange={(isOpen) => !isOpen && setConfirmCap(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <IconAlertTriangle className="size-5 text-danger" />
                {t('capability.confirmTitle')}
              </DialogTitle>
              <DialogDescription>
                {confirmCap.description ?? t('capability.confirmBody', { label: confirmCap.label })}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setConfirmCap(null)}>
                {t('capability.cancel')}
              </Button>
              <Button
                variant="destructive"
                onClick={() => {
                  const cap = confirmCap;
                  setConfirmCap(null);
                  if (cap) void invoke(cap);
                }}
              >
                <IconLock className="size-3.5" />
                {t('capability.confirm')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
