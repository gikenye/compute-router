# Base image confirmed against Cloudflare's own docs (Dockerfile
# reference page). Version tag: check
# https://developers.cloudflare.com/sandbox/configuration/dockerfile/
# for the current recommended tag before deploying — SDK is Beta and
# moves fast; 0.7.0 was current as of this repo's design phase.
FROM docker.io/cloudflare/sandbox:0.7.0

# MVP template: "node-build". Base image already includes Node.js 20 LTS
# + npm + Bun per Cloudflare's docs. Add whatever else your target
# workload needs below (this is the ONE image the MVP ships — see
# SPEC-100 template-selection note: Cloudflare fixes the image at
# Worker deploy time, it is not selectable per-request).
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    git \
    && rm -rf /var/lib/apt/lists/*
