// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import { ct } from '../canvas-theme';
import type { EditGroupDialogState } from './useCanvasContextMenus';

const GROUP_COLORS = ['#3b82f6', '#8b5cf6', '#f59e0b', '#10b981', '#ef4444', '#ec4899', '#06b6d4', '#f97316'];

interface EditGroupDialogProps {
  dialog: NonNullable<EditGroupDialogState>;
  onChange: (dialog: NonNullable<EditGroupDialogState>) => void;
  onCommit: () => void;
  onCancel: () => void;
  inputRef: React.RefObject<HTMLInputElement | null>;
}

export function EditGroupDialog({ dialog, onChange, onCommit, onCancel, inputRef }: EditGroupDialogProps) {
  const { t } = useTranslation('common');

  return (
    <div className={`absolute inset-0 z-50 flex items-center justify-center ${ct.backdrop}`}>
      <div className="w-72 rounded-lg border border-border bg-card p-4 shadow-xl" onMouseDown={(e) => e.stopPropagation()}>
        <p className="mb-3 text-xs font-medium text-foreground">{t('workflows.editGroup')}</p>
        <label className="mb-1 block text-[10px] text-muted-foreground">{t('workflows.groupNameLabel')}</label>
        <input
          ref={inputRef}
          value={dialog.name}
          onChange={(e) => onChange({ ...dialog, name: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === 'Enter') onCommit();
            if (e.key === 'Escape') onCancel();
          }}
          placeholder={t('workflows.groupName')}
          className="mb-3 h-8 w-full rounded-md border border-input bg-transparent px-2 text-xs text-foreground outline-none focus:ring-2 focus:ring-ring"
        />
        <label className="mb-1.5 block text-[10px] text-muted-foreground">{t('workflows.groupColorLabel')}</label>
        <div className="mb-4 flex flex-wrap gap-1.5">
          {GROUP_COLORS.map((c) => (
            // Raw <button> kept: color swatch with dynamic inline backgroundColor doesn't fit Button's API
            <button
              key={c}
              aria-label={t('workflows.selectColor', { color: c })}
              onClick={() => onChange({ ...dialog, color: c })}
              className={cn(
                'size-6 rounded-md border-2 transition-all',
                dialog.color === c
                  ? 'border-white scale-110'
                  : 'border-transparent hover:border-white/30',
              )}
              style={{ backgroundColor: c }}
            />
          ))}
        </div>
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={onCancel}
          >
            {t('actions.cancel')}
          </Button>
          <Button
            size="sm"
            onClick={onCommit}
          >
            {t('actions.save')}
          </Button>
        </div>
      </div>
    </div>
  );
}

