// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin

import "errors"

// ErrAIProviderNotLoaded is returned when a chat is routed to a provider
// type whose plugin adapter is not loaded. It lives in core so both the
// ai_providers module (which resolves connections) and the ai_assistant
// module (which drives the conversation) can share one sentinel without
// importing each other.
var ErrAIProviderNotLoaded = errors.New("ai provider plugin not loaded")
