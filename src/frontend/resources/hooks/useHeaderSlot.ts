// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// useHeaderSlot — resolve the topbar's portal target.
//
// `#header-slot` lives in Header, which mounts in the same commit as the
// routed page. Reading it during render is a coin flip, so we wait one frame
// and then re-render with the element. Every page that wants to put something
// in the topbar goes through here rather than calling getElementById inline,
// which is what five detail pages currently do.

import { useEffect, useState } from 'react';

export function useHeaderSlot(id = 'header-slot'): HTMLElement | null {
  const [slot, setSlot] = useState<HTMLElement | null>(null);

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      setSlot(document.getElementById(id));
    });
    return () => {
      cancelAnimationFrame(frame);
    };
  }, [id]);

  return slot;
}
