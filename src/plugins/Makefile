# McHarbor-AddOns top-level Makefile.
#
# Plugin authors usually `cd Plugins/<slug> && make`. This top-level
# Makefile is for repo-wide chores: re-signing the catalog, linting
# all plugins, and producing a release tarball.

SHELL := /usr/bin/env bash
GO    ?= go

PLUGIN_DIRS := $(wildcard Plugins/*/Makefile)

.PHONY: help list build sign all clean lint sign-index verify-index

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

list: ## List every plugin folder.
	@for d in $(PLUGIN_DIRS); do echo "$$d"; done

build: ## Build every plugin (delegates to each plugin's Makefile).
	@for mk in $(PLUGIN_DIRS); do \
		echo "==> $$mk"; \
		$(MAKE) -f $$mk build || exit 1; \
	done

sign: build ## Build + sign every plugin.
	@for mk in $(PLUGIN_DIRS); do \
		echo "==> $$mk"; \
		$(MAKE) -f $$mk sign || exit 1; \
	done

sign-index: ## Re-sign index.yaml (requires signing/key.pem).
	@python3 scripts/sign-index.py index.yaml signing/key.pem

verify-index: ## Verify the signature on index.yaml.
	@python3 scripts/verify-index.py index.yaml signing/key.pub

lint: ## Lint every plugin (delegates).
	@for mk in $(PLUGIN_DIRS); do \
		echo "==> $$mk"; \
		$(MAKE) -f $$mk lint || exit 1; \
	done

clean: ## Remove all build outputs.
	@find Plugins -type d -name dist -exec rm -rf {} + 2>/dev/null || true
	@find . -maxdepth 4 -name '*.so' -delete 2>/dev/null || true
	@find . -maxdepth 4 -name '*.dll' -delete 2>/dev/null || true
	@find . -maxdepth 4 -name '*.tar.gz' -not -path './Apps/*' -delete 2>/dev/null || true
	@echo "cleaned"