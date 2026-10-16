// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none (ConfirmDialog reads `common.actions.*`).

import { useState } from 'react';
import { ConfirmDialog } from './ConfirmDialog';
import { Button } from './Button';

export default {
  title: 'Primitives / ConfirmDialog',
};

type DemoProps = {
  triggerLabel: string;
  triggerVariant: 'destructive' | 'outline' | 'default';
  title: string;
  description: string;
  variant: 'destructive' | 'caution' | 'default';
  confirmLabel: string;
};

function Demo({ triggerLabel, triggerVariant, ...dialog }: DemoProps) {
  const [isOpen, setIsOpen] = useState(false);
  return (
    <div className="p-8">
      <Button variant={triggerVariant} onClick={() => setIsOpen(true)}>
        {triggerLabel}
      </Button>
      <ConfirmDialog open={isOpen} onOpenChange={setIsOpen} onConfirm={() => setIsOpen(false)} {...dialog} />
    </div>
  );
}

export const Destructive = () => (
  <Demo
    triggerLabel="Remove container"
    triggerVariant="destructive"
    title="Remove container?"
    description="This permanently removes the container and its anonymous volumes. Image layers are preserved."
    variant="destructive"
    confirmLabel="Remove"
  />
);

export const Caution = () => (
  <Demo
    triggerLabel="Force redeploy"
    triggerVariant="outline"
    title="Force redeploy stack?"
    description="All running services will be stopped and recreated with the current configuration. Data volumes are not affected."
    variant="caution"
    confirmLabel="Redeploy"
  />
);

export const Default = () => (
  <Demo
    triggerLabel="Apply changes"
    triggerVariant="default"
    title="Apply changes?"
    description="The updated configuration is applied to the selected environment."
    variant="default"
    confirmLabel="Apply"
  />
);
