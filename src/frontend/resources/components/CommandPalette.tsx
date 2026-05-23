// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { Command } from "cmdk";
import {
  IconLayoutDashboard,
  IconBox,
  IconPhoto,
  IconDeviceFloppy,
  IconNetwork,
  IconStack2,
  IconTerminal,
  IconFileText,
  IconWorld,
  IconBook,
  IconGitBranch,
  IconActivity,
  IconClipboardList,
  IconSettings,
  IconRefresh,
  IconInfoCircle,
} from "@tabler/icons-react";

const commands = [
  { labelKey: "nav.dashboard", to: "/dashboard", icon: IconLayoutDashboard },
  { labelKey: "nav.containers", to: "/containers", icon: IconBox },
  { labelKey: "nav.images", to: "/images", icon: IconPhoto },
  { labelKey: "nav.volumes", to: "/volumes", icon: IconDeviceFloppy },
  { labelKey: "nav.networks", to: "/networks", icon: IconNetwork },
  { labelKey: "nav.stacks", to: "/stacks", icon: IconStack2 },
  { labelKey: "nav.terminal", to: "/terminal", icon: IconTerminal },
  { labelKey: "nav.logs", to: "/logs", icon: IconFileText },
  { labelKey: "nav.environments", to: "/environments", icon: IconWorld },
  { labelKey: "nav.blueprints", to: "/blueprints", icon: IconBook },
  { labelKey: "nav.git", to: "/git", icon: IconGitBranch },
  { labelKey: "nav.reconciler", to: "/reconciler", icon: IconRefresh },
  { labelKey: "nav.activity", to: "/activity", icon: IconActivity },
  { labelKey: "nav.audit", to: "/audit", icon: IconClipboardList },
  { labelKey: "nav.settings", to: "/settings", icon: IconSettings },
  {
    labelKey: "about.menuItem",
    to: "/settings?tab=about",
    icon: IconInfoCircle,
  },
];

type CommandPaletteProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const { t } = useTranslation("common");
  const navigate = useNavigate();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onOpenChange]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh]"
      onClick={() => onOpenChange(false)}
    >
      <div className="fixed inset-0 bg-background/80 backdrop-blur-sm" />
      <div
        className="relative z-10 w-full max-w-lg rounded-lg border border-border bg-popover shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <Command className="flex flex-col">
          <Command.Input
            placeholder={t("commandPalette.placeholder")}
            className="w-full border-b border-border bg-transparent px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
            autoFocus
          />
          <Command.List className="max-h-80 overflow-y-auto p-2">
            <Command.Empty className="px-4 py-6 text-center text-sm text-muted-foreground">
              {t("commandPalette.noResults")}
            </Command.Empty>
            <Command.Group
              heading={t("nav.navigation")}
              className="px-2 py-1 text-xs font-medium text-muted-foreground"
            >
              {commands.map((cmd) => {
                const label = t(cmd.labelKey);
                return (
                  <Command.Item
                    key={cmd.to}
                    value={label}
                    onSelect={() => {
                      navigate(cmd.to);
                      onOpenChange(false);
                    }}
                    className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                  >
                    <cmd.icon className="h-4 w-4 text-muted-foreground" />
                    {label}
                  </Command.Item>
                );
              })}
            </Command.Group>
          </Command.List>
        </Command>
      </div>
    </div>
  );
}
