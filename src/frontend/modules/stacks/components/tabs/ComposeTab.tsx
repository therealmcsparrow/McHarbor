// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPencil, IconDeviceFloppy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { CodeEditor } from '@resources/components/CodeEditor';
import { useStackCompose, useUpdateStack } from '../../hooks/useStacks';

type ComposeTabProps = {
  stackName: string;
  isManaged: boolean;
  editing?: boolean;
};

export function ComposeTab({ stackName, isManaged, editing: globalEditing }: ComposeTabProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const { data: content, isLoading } = useStackCompose(stackName);
  const updateStack = useUpdateStack();
  const [localEditing, setLocalEditing] = useState(false);
  const [editContent, setEditContent] = useState('');

  const isEditing = globalEditing || localEditing;

  useEffect(() => {
    if (content !== undefined) setEditContent(content);
  }, [content]);

  useEffect(() => {
    if (globalEditing && content !== undefined) {
      setEditContent(content);
    }
  }, [globalEditing, content]);

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
          {!isManaged && (
            <Badge variant="secondary" className="text-[10px]">
              {t('detail.readOnly')}
            </Badge>
          )}
          {isManaged && !isEditing && (
            <Button variant="outline" size="sm" onClick={() => setLocalEditing(true)}>
              <IconPencil className="h-3.5 w-3.5" />
              {t('detail.editCompose')}
            </Button>
          )}
          {isManaged && isEditing && (
            <>
              {!globalEditing && (
                <Button variant="outline" size="sm" onClick={() => setLocalEditing(false)}>
                  {tc('actions.cancel')}
                </Button>
              )}
              <Button size="sm" onClick={handleSave} disabled={updateStack.isPending}>
                <IconDeviceFloppy className="h-3.5 w-3.5" />
                {updateStack.isPending ? t('edit.saving') : t('detail.saveCompose')}
              </Button>
            </>
          )}
        </div>
      </div>

      <CodeEditor
        value={isEditing ? editContent : content ?? ''}
        onChange={isEditing ? setEditContent : undefined}
        readOnly={!isEditing}
        language="yaml"
        className="min-h-0 flex-1"
      />
    </div>
  );
}

