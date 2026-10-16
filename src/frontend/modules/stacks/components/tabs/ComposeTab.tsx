// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPencil, IconDeviceFloppy, IconEye, IconEyeOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { CodeEditor } from '@resources/components/CodeEditor';
import { useStackCompose, useStackComposeRevealed, useUpdateStack } from '@resources/hooks/useStacks';
import { PERM } from '@core/auth/permissionKeys';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';

type ComposeTabProps = {
  stackName: string;
  isManaged: boolean;
  editing?: boolean;
  readOnly?: boolean;
};

export function ComposeTab({ stackName, isManaged, editing: globalEditing, readOnly = false }: ComposeTabProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const cancelLabel = tc('actions.cancel');
  const { data: content, isLoading } = useStackCompose(stackName);
  const updateStack = useUpdateStack();
  const [localEditing, setLocalEditing] = useState(false);
  const [editContent, setEditContent] = useState(content ?? '');
  const [revealed, setRevealed] = useState(false);

  const isEditing = !readOnly && (globalEditing || localEditing);
  // Editing always starts from the masked text; the server puts back every value that still
  // carries the marker, so the plaintext never has to round-trip through the editor.
  const hasMasked = (content ?? '').includes(ENV_REDACTION_MARKER);
  const revealQuery = useStackComposeRevealed(stackName, revealed && !isEditing);
  const shownContent = revealed && revealQuery.data !== undefined ? revealQuery.data : (content ?? '');

  const [seededFrom, setSeededFrom] = useState({ content, globalEditing });
  if (seededFrom.content !== content || seededFrom.globalEditing !== globalEditing) {
    setSeededFrom({ content, globalEditing });
    if (content !== undefined) setEditContent(content);
  }

  const handleSave = () => {
    updateStack.mutate(
      { name: stackName, compose: editContent },
      {
        onSuccess: () => setLocalEditing(false),
      },
    );
  };

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">{t('detail.composeFile')}</h3>
        <div className="flex items-center gap-2">
          {(!isManaged || readOnly) && (
            <Badge variant="secondary" className="text-[10px]">
              {t('detail.readOnly')}
            </Badge>
          )}
          {!isEditing && (hasMasked || revealed) && (
            <Button
              variant="outline"
              size="sm"
              permission={PERM.STACKS_MANAGE}
              disabled={revealed && revealQuery.isFetching}
              onClick={() => setRevealed((value) => !value)}
            >
              {revealed ? <IconEyeOff className="h-3.5 w-3.5" /> : <IconEye className="h-3.5 w-3.5" />}
              {revealed ? t('detail.hideComposeValues') : t('detail.revealComposeValues')}
            </Button>
          )}
          {isManaged && !readOnly && !isEditing && (
            <Button variant="outline" size="sm" onClick={() => setLocalEditing(true)}>
              <IconPencil className="h-3.5 w-3.5" />
              {t('detail.editCompose')}
            </Button>
          )}
          {isManaged && !readOnly && isEditing && (
            <>
              {!globalEditing && (
                <Button variant="outline" size="sm" onClick={() => setLocalEditing(false)}>
                  {cancelLabel}
                </Button>
              )}
              <Button permission={PERM.STACKS_MANAGE} size="sm" onClick={handleSave} disabled={updateStack.isPending}>
                <IconDeviceFloppy className="h-3.5 w-3.5" />
                {updateStack.isPending ? t('edit.saving') : t('detail.saveCompose')}
              </Button>
            </>
          )}
        </div>
      </div>

      <CodeEditor
        value={isEditing ? editContent : shownContent}
        onChange={isEditing ? setEditContent : undefined}
        readOnly={!isEditing}
        language="yaml"
        className="min-h-0 flex-1"
      />
    </div>
  );
}
