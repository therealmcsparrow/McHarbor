// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEdgeContextMenu } from './useEdgeContextMenu';
import { useNodeContextMenu } from './useNodeContextMenu';
import { useGroupContextMenu } from './useGroupContextMenu';

export type { GroupDialogState, EditGroupDialogState } from './useGroupContextMenu';

export function useCanvasContextMenus(onExecute?: (triggerNodeId: string) => void) {
  const edge = useEdgeContextMenu();
  const node = useNodeContextMenu(onExecute);
  const group = useGroupContextMenu();

  return {
    ...edge,
    ...node,
    ...group,
  };
}
