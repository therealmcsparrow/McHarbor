// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ImageInfo = {
  Id: string;
  ParentId: string;
  RepoTags: string[] | null;
  RepoDigests: string[] | null;
  Created: number;
  Size: number;
  SharedSize: number;
  VirtualSize: number;
  Labels: Record<string, string> | null;
  Containers: number;
  Protected?: boolean;
  UsedBy?: string[];
};

export type ImageInspect = {
  Id: string;
  RepoTags: string[] | null;
  RepoDigests: string[] | null;
  Parent: string;
  Created: string;
  DockerVersion: string;
  Author: string;
  Config: {
    Env: string[] | null;
    Cmd: string[] | null;
    Entrypoint: string[] | null;
    ExposedPorts: Record<string, object> | null;
    Volumes: Record<string, object> | null;
    Labels: Record<string, string> | null;
    WorkingDir: string;
    User: string;
    StopSignal: string;
  };
  Architecture: string;
  Variant: string;
  Os: string;
  OsVersion: string;
  Size: number;
  RootFS: {
    Type: string;
    Layers: string[] | null;
  };
  GraphDriver: {
    Name: string;
    Data: Record<string, string> | null;
  };
  Metadata: {
    LastTagTime: string;
  };
  Protected?: boolean;
};

export type ImageHistoryItem = {
  Comment: string;
  Created: number;
  CreatedBy: string;
  Id: string;
  Size: number;
  Tags: string[] | null;
};
