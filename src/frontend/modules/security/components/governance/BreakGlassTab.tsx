// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { BreakGlassAdminPanel } from './BreakGlassAdminPanel';
import { BreakGlassSelfPanel } from './BreakGlassSelfPanel';

export function BreakGlassTab() {
  return (
    <div className="space-y-6">
      <BreakGlassSelfPanel />
      <BreakGlassAdminPanel />
    </div>
  );
}
