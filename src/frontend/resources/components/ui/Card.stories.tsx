// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from './Card';

export default {
  title: 'Primitives / Card',
};

export const Default = () => (
  <div className="p-8">
    <Card className="max-w-sm">
      <CardHeader>
        <CardTitle>Container status</CardTitle>
        <CardDescription>Live readouts from the local Docker daemon.</CardDescription>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-foreground">12 containers running, 3 stopped.</p>
      </CardContent>
      <CardFooter>
        <span className="text-xs text-muted-foreground">Updated 2 minutes ago</span>
      </CardFooter>
    </Card>
  </div>
);

export const BareCard = () => (
  <div className="p-8">
    <Card className="max-w-sm p-6">
      <p className="text-sm text-foreground">
        Cards without a header / footer just render a rounded surface. Use this for quick grouping when there's no clear
        title.
      </p>
    </Card>
  </div>
);
