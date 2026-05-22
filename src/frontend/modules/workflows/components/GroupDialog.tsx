// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { ct } from '../canvas-theme';

interface GroupDialogProps {
  groupName: string;
  onGroupNameChange: (name: string) => void;
  onCommit: () => void;
  onCancel: () => void;
  inputRef: React.RefObject<HTMLInputElement | null>;
}

export function GroupDialog({ groupName, onGroupNameChange, onCommit, onCancel, inputRef }: GroupDialogProps) {
  const { t } = useTranslation('common');

  return (
    <div className={`absolute inset-0 z-50 flex items-center justify-center ${ct.backdrop}`}>
      <div className="rounded-lg border border-border bg-card p-4 shadow-xl" onMouseDown={(e) => e.stopPropagation()}>
        <p className="mb-2 text-xs font-medium text-foreground">{t('workflows.groupName')}</p>
        <input
          ref={inputRef}
          value={groupName}
          onChange={(e) => onGroupNameChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') onCommit();
            if (e.key === 'Escape') onCancel();
          }}
          placeholder={t('workflows.groupName')}
          className="mb-3 h-8 w-56 rounded-md border border-input bg-transparent px-2 text-xs text-foreground outline-none focus:ring-2 focus:ring-ring"
        />
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
            {t('actions.create')}
          </Button>
        </div>
      </div>
    </div>
  );
}

