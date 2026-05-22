# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

.PHONY: check-protocol

# Verify server and agent protocol files are in sync.
# Strips package declarations, all comments (full-line and inline), blank lines,
# and normalizes whitespace so cosmetic differences don't cause false positives.
PROTO_NORMALIZE = grep -v '^package ' | sed 's|[[:space:]]*//.*$$||' | sed '/^[[:space:]]*$$/d' | sed 's/\t/ /g' | tr -s ' '

check-protocol:
	@echo "Checking protocol sync..."
	@diff <(cat src/backend/core/agent/protocol.go | $(PROTO_NORMALIZE)) \
	      <(cat src/agent/protocol.go | $(PROTO_NORMALIZE)) \
	  && echo "Protocol files in sync ✓" \
	  || (echo "ERROR: Protocol files diverged! Update both files." && exit 1)
