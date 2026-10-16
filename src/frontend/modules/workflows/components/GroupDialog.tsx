// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { useFocusTrap } from '@resources/hooks/useFocusTrap';
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
  const titleID = useId();
  const panelRef = useFocusTrap<HTMLDialogElement>({ active: true, restoreFocus: true });

  // Deliberately not the shared Dialog: Radix portals to document.body, and
  // this backdrop belongs to the canvas rather than to the editor around it.
  // What the shared component would have brought with it is here instead.
  return (
    <div className={`absolute inset-0 z-50 flex items-center justify-center ${ct.backdrop}`}>
      <dialog
        open
        ref={panelRef}
        aria-modal="true"
        aria-labelledby={titleID}
        className="static m-0 rounded-lg border border-border bg-card p-4 text-foreground shadow-xl"
      >
        <div
          role="presentation"
          onMouseDown={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onCancel();
          }}
        >
          <p id={titleID} className="mb-2 text-xs font-medium text-foreground">
            {t('workflows.groupName')}
          </p>
          <input
            ref={inputRef}
            value={groupName}
            onChange={(e) => onGroupNameChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') onCommit();
              if (e.key === 'Escape') onCancel();
            }}
            placeholder={t('workflows.groupName')}
            className="mc-control mb-3 w-56 rounded-md border border-input bg-transparent px-2 text-xs text-foreground outline-none focus:ring-2 focus:ring-ring"
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={onCancel}>
              {t('actions.cancel')}
            </Button>
            <Button size="sm" onClick={onCommit}>
              {t('actions.create')}
            </Button>
          </div>
        </div>
      </dialog>
    </div>
  );
}
