// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';

type UploadProgressProps = {
  progress: number | null;
};

export function UploadProgress({ progress }: UploadProgressProps) {
  const { t } = useTranslation('containers');

  return (
    <div className="space-y-1.5">
      <progress
        className="h-2 w-full overflow-hidden rounded-full [&::-moz-progress-bar]:bg-primary [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-primary"
        max={100}
        value={progress ?? undefined}
      />
      <p className="text-xs text-muted-foreground">
        {progress === null ? t('files.uploading') : t('files.uploadProgress', { percent: progress })}
      </p>
    </div>
  );
}
