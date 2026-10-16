// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import type { StackInfo } from '@resources/hooks/useStacks';

export function useStacksPageDialogs() {
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<StackInfo | null>(null);
  const [logsTarget, setLogsTarget] = useState<StackInfo | null>(null);
  const [removeTarget, setRemoveTarget] = useState<StackInfo | null>(null);
  const [takeOverTarget, setTakeOverTarget] = useState<StackInfo | null>(null);
  const [downTarget, setDownTarget] = useState<StackInfo | null>(null);
  const [reinstallAllConfirmOpen, setReinstallAllConfirmOpen] = useState(false);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);

  return {
    createOpen,
    setCreateOpen,
    editTarget,
    setEditTarget,
    logsTarget,
    setLogsTarget,
    removeTarget,
    setRemoveTarget,
    takeOverTarget,
    setTakeOverTarget,
    downTarget,
    setDownTarget,
    reinstallAllConfirmOpen,
    setReinstallAllConfirmOpen,
    pruneConfirmOpen,
    setPruneConfirmOpen,
  };
}

export type StacksPageDialogState = ReturnType<typeof useStacksPageDialogs>;
