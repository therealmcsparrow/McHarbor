// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Tabs, TabsList, TabsTrigger, TabsContent } from './Tabs';
import { Box } from './Layout';

export default {
  title: 'Primitives / Tabs',
};

export const Basic = () => (
  <Box className="max-w-2xl p-8">
    <Tabs defaultValue="overview">
      <TabsList>
        <TabsTrigger value="overview">Overview</TabsTrigger>
        <TabsTrigger value="logs">Logs</TabsTrigger>
        <TabsTrigger value="inspect">Inspect</TabsTrigger>
        <TabsTrigger value="terminal">Terminal</TabsTrigger>
      </TabsList>
      <TabsContent value="overview">
        <p className="text-sm text-muted-foreground">Container summary, resource usage, and quick actions.</p>
      </TabsContent>
      <TabsContent value="logs">
        <p className="text-sm text-muted-foreground">Tailing the most recent log lines.</p>
      </TabsContent>
      <TabsContent value="inspect">
        <p className="text-sm text-muted-foreground">Raw container config as JSON.</p>
      </TabsContent>
      <TabsContent value="terminal">
        <p className="text-sm text-muted-foreground">Attach an interactive shell.</p>
      </TabsContent>
    </Tabs>
  </Box>
);

export const Disabled = () => (
  <Box className="max-w-2xl p-8">
    <Tabs defaultValue="summary">
      <TabsList>
        <TabsTrigger value="summary">Summary</TabsTrigger>
        <TabsTrigger value="advanced" disabled>
          Advanced
        </TabsTrigger>
        <TabsTrigger value="raw">Raw</TabsTrigger>
      </TabsList>
      <TabsContent value="summary">Visible summary content.</TabsContent>
      <TabsContent value="advanced">Hidden because the trigger is disabled.</TabsContent>
      <TabsContent value="raw">Raw config dump.</TabsContent>
    </Tabs>
  </Box>
);
