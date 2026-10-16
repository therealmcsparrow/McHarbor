// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type PluginRoute = {
  slug: string;
  method: string;
  path: string;
  public: boolean;
};

export function routesBlockLines(manifestText: string): string[];
export function extractRoutes(manifestText: string, manifestPath: string): Omit<PluginRoute, 'slug'>[];
export function collectPluginRoutes(pluginsRoot?: string): PluginRoute[];
