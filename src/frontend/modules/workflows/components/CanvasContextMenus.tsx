// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { WorkflowContextMenu } from './WorkflowContextMenu';
import type { useCanvasContextMenus } from './useCanvasContextMenus';

interface CanvasContextMenusProps {
  menus: ReturnType<typeof useCanvasContextMenus>;
}

export function CanvasContextMenus({ menus }: CanvasContextMenusProps) {
  return (
    <>
      {/* Edge context menu */}
      {menus.contextMenu && (
        <WorkflowContextMenu
          x={menus.contextMenu.x}
          y={menus.contextMenu.y}
          items={menus.edgeContextMenuItems()}
          onSelect={menus.onContextMenuSelect}
          onClose={() => menus.setContextMenu(null)}
        />
      )}

      {/* Node context menu */}
      {menus.nodeContextMenu && (
        <WorkflowContextMenu
          x={menus.nodeContextMenu.x}
          y={menus.nodeContextMenu.y}
          items={menus.nodeContextMenuItems()}
          onSelect={menus.onNodeContextMenuSelect}
          onClose={() => menus.setNodeContextMenu(null)}
        />
      )}

      {/* Port context menu */}
      {menus.portContextMenu && (
        <WorkflowContextMenu
          x={menus.portContextMenu.x}
          y={menus.portContextMenu.y}
          items={menus.portContextMenuItems()}
          onSelect={menus.onPortContextMenuSelect}
          onClose={() => menus.setPortContextMenu(null)}
        />
      )}

      {/* Canvas context menu (for grouping + alignment) */}
      {menus.canvasContextMenu && (
        <WorkflowContextMenu
          x={menus.canvasContextMenu.x}
          y={menus.canvasContextMenu.y}
          items={menus.canvasContextMenuItems()}
          onSelect={menus.onCanvasContextMenuSelect}
          onClose={() => menus.setCanvasContextMenu(null)}
        />
      )}

      {/* Group context menu */}
      {menus.groupContextMenu && (
        <WorkflowContextMenu
          x={menus.groupContextMenu.x}
          y={menus.groupContextMenu.y}
          items={menus.groupContextMenuItems()}
          onSelect={menus.onGroupContextMenuSelect}
          onClose={() => menus.setGroupContextMenu(null)}
        />
      )}
    </>
  );
}
