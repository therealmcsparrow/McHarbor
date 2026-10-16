// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RouterProvider } from 'react-router';
import { router } from '@core/router';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { McHarborRestartDialog } from '@resources/components/McHarborRestartDialog';

export default function App() {
  return (
    <TooltipProvider delayDuration={300}>
      <RouterProvider router={router} />
      <McHarborRestartDialog />
    </TooltipProvider>
  );
}
