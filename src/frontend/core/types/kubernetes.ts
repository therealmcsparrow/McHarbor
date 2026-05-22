// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type PodSummary = {
  name: string;
  namespace: string;
  status: string;
  ready: string;
  restarts: number;
  age: string;
  ip: string;
  node: string;
  labels?: Record<string, string>;
};

export type PodDetail = PodSummary & {
  containers: PodContainerInfo[];
  conditions?: PodCondition[];
  createdAt: string;
};

export type PodContainerInfo = {
  name: string;
  image: string;
  ready: boolean;
  restartCount: number;
  state: string;
};

export type PodCondition = {
  type: string;
  status: string;
};

export type DeploymentSummary = {
  name: string;
  namespace: string;
  ready: string;
  upToDate: number;
  available: number;
  age: string;
  images: string[];
  labels?: Record<string, string>;
  replicas: number;
  desiredReplicas: number;
};

export type DeploymentDetail = DeploymentSummary & {
  strategy: string;
  conditions?: DeploymentCondition[];
  createdAt: string;
};

export type DeploymentCondition = {
  type: string;
  status: string;
  message?: string;
};

export type K8sServiceSummary = {
  name: string;
  namespace: string;
  type: string;
  clusterIP: string;
  externalIP?: string;
  ports: K8sServicePort[];
  age: string;
  labels?: Record<string, string>;
  selector?: Record<string, string>;
};

export type K8sServicePort = {
  name?: string;
  protocol: string;
  port: number;
  targetPort: string;
  nodePort?: number;
};

export type NamespaceSummary = {
  name: string;
  status: string;
  age: string;
  labels?: Record<string, string>;
  createdAt: string;
};
