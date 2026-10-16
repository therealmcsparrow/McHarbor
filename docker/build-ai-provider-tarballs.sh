#!/bin/sh
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Build helper invoked from docker/Dockerfile's backend stage.
# Builds the AI provider plugins (openai, anthropic, ollama,
# minimax, mistral, plus the hosted and self-hosted OpenAI-compatible
# providers listed in SLUGS) into /plugins/dist/<slug>/backend/<slug>.so and
# packages each into /plugins/dist/mcharbor-plugin-<slug>-<version>.tar.gz.
#
# Lives in a separate file because busybox sh (the default `/bin/sh`
# in alpine) parses for-loops more strictly than bash and the inline
# Dockerfile form with multi-line backslash continuations trips its
# parser. A standalone script avoids the issue.

set -eu

cd /app

SLUGS="ai-provider-openai ai-provider-anthropic ai-provider-ollama ai-provider-minimax ai-provider-mistral ai-provider-gemini ai-provider-deepseek ai-provider-xai ai-provider-groq ai-provider-openrouter ai-provider-perplexity ai-provider-cohere ai-provider-together ai-provider-fireworks ai-provider-lmstudio ai-provider-llamacpp ai-provider-vllm ai-provider-localai"

# Stage 1: build each plugin's .so in the host's module context and
# write its tarball into /plugins/dist/.
for slug in $SLUGS; do
    echo "==> building $slug"

    rm -rf /app/_plugin_build
    mkdir -p /app/_plugin_build/$slug
    cp -R /plugins/Plugins/$slug/backend /app/_plugin_build/$slug/
    rm -f /app/_plugin_build/$slug/backend/go.mod /app/_plugin_build/$slug/backend/go.sum

    CGO_ENABLED=1 go build -buildmode=plugin \
        -o /plugins/dist/$slug/backend/$slug.so \
        /app/_plugin_build/$slug/backend

    rm -rf /app/_plugin_build

    mkdir -p /plugins/dist/$slug
    cp /plugins/Plugins/$slug/plugin.yaml /plugins/dist/$slug/plugin.yaml
    if [ -d /plugins/Plugins/$slug/i18n ]; then
        mkdir -p /plugins/dist/$slug/i18n
        cp -R /plugins/Plugins/$slug/i18n/. /plugins/dist/$slug/i18n/
    fi

    VERSION=$(awk '/^version:[[:space:]]*/{print $2; exit}' /plugins/Plugins/$slug/plugin.yaml | tr -d '"')
    # Tarball filename uses the bare `<slug>-<version>.tar.gz` form
    # (NOT `mcharbor-plugin-<slug>-<version>.tar.gz` as the per-plugin
    # Makefile does) because the dev catalog's `download_url` field
    # expects the bare name. The signed release tarball published
    # to the public index uses the longer `mcharbor-plugin-` form.
    tar -C /plugins/dist/$slug -czf /plugins/dist/$slug-$VERSION.tar.gz .
done

echo "==> all AI provider tarballs built"

# Stage 2 (only runs when invoked from plugin-packager-ai): collect
# the packaged tarballs into /packaged/ so docker buildx can extract
# them from the build output. The first stage above is a no-op for
# plugin-packager-ai because /plugins/dist/ already exists in the
# backend stage's filesystem.
if [ -d /packaged ] || [ "${MCHARBOR_COLLECT_AI:-0}" = "1" ]; then
    mkdir -p /packaged
    # Stage 1 writes `/plugins/dist/<slug>-<version>.tar.gz` (the bare
    # name — see the comment at the top of stage 1). Match that here.
    for t in /plugins/dist/ai-provider-*.tar.gz; do
        cp "$t" /packaged/
    done
    echo "==> AI provider tarballs staged in /packaged"
fi

