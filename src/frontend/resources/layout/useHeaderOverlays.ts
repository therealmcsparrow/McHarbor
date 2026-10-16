// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useSidebarStore } from '@resources/stores/sidebar';
import { isEditableTarget, isInCaptureZone } from '@resources/utils/keyboardCapture';

export type HeaderOverlayState = {
  searchOpen: boolean;
  setSearchOpen: (next: boolean | ((current: boolean) => boolean)) => void;
  containerOverviewOpen: boolean;
  setContainerOverviewOpen: (next: boolean) => void;
  imageOverviewOpen: boolean;
  setImageOverviewOpen: (next: boolean) => void;
  volumeOverviewOpen: boolean;
  setVolumeOverviewOpen: (next: boolean) => void;
  stackOverviewOpen: boolean;
  setStackOverviewOpen: (next: boolean) => void;
};

export function useHeaderOverlays(): HeaderOverlayState {
  const toggleSidebar = useSidebarStore((s) => s.toggle);
  const [searchOpen, setSearchOpen] = useState(false);
  const [containerOverviewOpen, setContainerOverviewOpen] = useState(false);
  const [imageOverviewOpen, setImageOverviewOpen] = useState(false);
  const [volumeOverviewOpen, setVolumeOverviewOpen] = useState(false);
  const [stackOverviewOpen, setStackOverviewOpen] = useState(false);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (isInCaptureZone(e.target)) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setSearchOpen((current) => !current);
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'b') {
        e.preventDefault();
        toggleSidebar();
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (isEditableTarget(e.target)) return;

      switch (e.key) {
        case 'c':
          e.preventDefault();
          setContainerOverviewOpen((prev) => !prev);
          break;
        case 'i':
          e.preventDefault();
          setImageOverviewOpen((prev) => !prev);
          break;
        case 'v':
          e.preventDefault();
          setVolumeOverviewOpen((prev) => !prev);
          break;
        case 's':
          e.preventDefault();
          setStackOverviewOpen((prev) => !prev);
          break;
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [toggleSidebar]);

  return {
    searchOpen,
    setSearchOpen,
    containerOverviewOpen,
    setContainerOverviewOpen,
    imageOverviewOpen,
    setImageOverviewOpen,
    volumeOverviewOpen,
    setVolumeOverviewOpen,
    stackOverviewOpen,
    setStackOverviewOpen,
  };
}
