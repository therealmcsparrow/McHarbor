// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PushDeliveryCard } from './PushDeliveryCard';
import { ToastDefaultsCard } from './ToastDefaultsCard';

/**
 * Settings → Notifications.
 *
 * The app-wide toast default: placement, duration, stacking and whether
 * accounts may pick their own. It used to close the General stack; it has a
 * section of its own now so it can be found by name rather than by scrolling.
 *
 * The per-account override is not here: it is a preference about the
 * signed-in user, so it lives on Profile → Notifications beside theme and
 * password. The defaults card links to it.
 */
export function NotificationsTab() {
  return (
    <div className="space-y-6">
      <ToastDefaultsCard />
      <PushDeliveryCard />
    </div>
  );
}
