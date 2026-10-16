// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { NotificationsCard } from '@resources/components/settings/NotificationsCard';

/**
 * Profile → Notifications.
 *
 * The card itself lives in the settings module because that is where the
 * toast hooks, the position picker and the shared knob fields are, and where
 * the app-wide default it overrides still renders. Only its home moved: this
 * is a preference about the signed-in account, so it belongs beside theme and
 * password rather than on an operator's settings page.
 *
 * `mc-glass` is passed in rather than baked into the card: on the settings
 * page the group sits on a glass panel already and a second layer would read
 * as a box in a box, while here it sits on the bare canvas next to the other
 * profile sections and needs the material of its own.
 */
export function NotificationsTab() {
  return (
    <div className="space-y-6">
      <NotificationsCard className="mc-glass" />
    </div>
  );
}
