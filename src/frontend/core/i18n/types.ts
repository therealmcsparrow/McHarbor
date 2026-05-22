// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import common from './locales/en/common.json';
import auth from './locales/en/auth.json';
import containers from './locales/en/containers.json';
import images from './locales/en/images.json';
import volumes from './locales/en/volumes.json';
import networks from './locales/en/networks.json';
import stacks from './locales/en/stacks.json';
import environments from './locales/en/environments.json';
import settings from './locales/en/settings.json';
import dashboard from './locales/en/dashboard.json';
import kubernetes from './locales/en/kubernetes.json';
import terminal from './locales/en/terminal.json';
import security from './locales/en/security.json';
import docker from './locales/en/docker.json';

export type StaticLocaleResources = {
  common: typeof common;
  auth: typeof auth;
  containers: typeof containers;
  images: typeof images;
  volumes: typeof volumes;
  networks: typeof networks;
  stacks: typeof stacks;
  environments: typeof environments;
  settings: typeof settings;
  dashboard: typeof dashboard;
  kubernetes: typeof kubernetes;
  terminal: typeof terminal;
  security: typeof security;
  docker: typeof docker;
};
