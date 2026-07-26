// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconExternalLink,
  IconRotate,
  IconEye,
  IconRefresh,
  IconX,
} from '@tabler/icons-react';
import { toast as sonnerToast } from 'sonner';
import { useTranslation } from 'react-i18next';

type ToastAction = {
  label: string;
  onClick: () => void;
  icon?: 'view' | 'undo' | 'refresh' | 'open' | 'close';
};

function renderIcon(name: ToastAction['icon']) {
  switch (name) {
    case 'view':
      return <IconEye className="size-3.5" />;
    case 'undo':
      return <IconRotate className="size-3.5" />;
    case 'refresh':
      return <IconRefresh className="size-3.5" />;
    case 'open':
      return <IconExternalLink className="size-3.5" />;
    case 'close':
      return <IconX className="size-3.5" />;
    default:
      return null;
  }
}

type SuccessOptions = {
  description?: string;
  action?: ToastAction;
  duration?: number;
};

export function useActionToasts() {
  const { t } = useTranslation('common');

  const success = (message: string, options: SuccessOptions = {}) => {
    return sonnerToast.success(message, {
      description: options.description,
      duration: options.duration ?? 4000,
      action: options.action
        ? {
            label: options.action.label,
            onClick: options.action.onClick,
          }
        : undefined,
    });
  };

  const error = (message: string, options: { action?: ToastAction; duration?: number } = {}) => {
    return sonnerToast.error(message, {
      duration: options.duration ?? 6000,
      action: options.action
        ? {
            label: options.action.label,
            onClick: options.action.onClick,
          }
        : undefined,
    });
  };

  const info = (message: string, options: SuccessOptions = {}) => {
    return sonnerToast(message, {
      description: options.description,
      duration: options.duration ?? 4000,
      action: options.action
        ? {
            label: options.action.label,
            onClick: options.action.onClick,
          }
        : undefined,
    });
  };

  const undoable = (opts: { message: string; onUndo: () => void; duration?: number }) => {
    return sonnerToast(opts.message, {
      duration: opts.duration ?? 6000,
      action: {
        label: t('actions.undo', { defaultValue: 'Undo' }),
        onClick: () => opts.onUndo(),
      },
    });
  };

  return { success, error, info, undoable, renderIcon };
}
