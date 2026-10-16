// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from './Dialog';
import { Button } from './Button';
import { Input } from './Input';
import { Textarea } from './Textarea';

export default {
  title: 'Primitives / Dialog',
};

export const Basic = () => (
  <div className="p-8">
    <Dialog>
      <DialogTrigger asChild>
        <Button>Open dialog</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Confirm settings change</DialogTitle>
          <DialogDescription>This will apply the new configuration to every connected environment.</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <p className="text-sm text-muted-foreground">
            Use the body slot for the main form / content. The footer sticks to the bottom.
          </p>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline">Cancel</Button>
          <Button>Apply</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
);

export const WithForm = () => (
  <div className="p-8">
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline">New environment</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add a remote environment</DialogTitle>
          <DialogDescription>Provide a name and the Docker socket URL of the remote host.</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label htmlFor="dialog-story-name" className="text-sm font-medium text-foreground">
                Name
              </label>
              <Input id="dialog-story-name" placeholder="prod-cluster" />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="dialog-story-socket" className="text-sm font-medium text-foreground">
                Socket URL
              </label>
              <Input id="dialog-story-socket" placeholder="tcp://10.0.0.5:2375" />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="dialog-story-notes" className="text-sm font-medium text-foreground">
                Notes
              </label>
              <Textarea id="dialog-story-notes" placeholder="Optional context for future you" />
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline">Cancel</Button>
          <Button>Save</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
);

export const NoExplicitBody = () => (
  <div className="p-8">
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="ghost">Open simple dialog</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Heads up</DialogTitle>
        </DialogHeader>
        <p className="px-4 py-4 text-sm text-muted-foreground">
          When no <code>DialogBody</code> is supplied the component wraps loose children in one automatically so the
          layout still pads correctly.
        </p>
        <DialogFooter>
          <Button>Got it</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
);
