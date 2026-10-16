// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useUpdateStack } from '@resources/hooks/useStacks';
import type { StackDetail } from '@resources/hooks/useStacks';

export function useStackEditState(stack: StackDetail | undefined) {
  const updateStack = useUpdateStack();
  const [editing, setEditing] = useState(false);
  const [editDescription, setEditDescription] = useState(stack?.description ?? '');
  const [appliedStack, setAppliedStack] = useState(stack);
  if (appliedStack !== stack) {
    setAppliedStack(stack);
    if (stack) setEditDescription(stack.description ?? '');
  }

  const handleStartEdit = useCallback(() => {
    if (!stack) return;
    setEditDescription(stack.description ?? '');
    setEditing(true);
  }, [stack]);

  const handleCancelEdit = useCallback(() => {
    setEditing(false);
    if (stack) {
      setEditDescription(stack.description ?? '');
    }
  }, [stack]);

  const handleSave = useCallback(() => {
    if (!stack) return;
    updateStack.mutate(
      {
        name: stack.name,
        description: editDescription || undefined,
      },
      {
        onSuccess: () => setEditing(false),
      },
    );
  }, [stack, editDescription, updateStack]);

  return {
    editing,
    editDescription,
    setEditDescription,
    handleStartEdit,
    handleCancelEdit,
    handleSave,
    saving: updateStack.isPending,
  };
}
