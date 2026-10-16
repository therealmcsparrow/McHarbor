// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Navigate, useParams } from 'react-router';

export default function RunbookEditRedirect() {
  const { id } = useParams();
  const target = !id || id === 'new' ? '/runbooks?new=1' : `/runbooks?edit=${encodeURIComponent(id)}`;
  return <Navigate to={target} replace />;
}
