# Changelog

## Unreleased

- Unified package naming on `gpurun-sandbox`: Go module `github.com/gpurun/gpurun-sandbox/sdk-go`,
  Maven `ai.gpurun:gpurun-sandbox`, npm and PyPI `gpurun-sandbox`.
- Added the NovitaBox `boxapi` control plane and `boxd` data plane to the Go and
  Java SDKs while keeping the existing sandbox API available.
- Added TypeScript and Python clients with full control-plane and data-plane
  coverage, plus release and CI integration.
- Added `GPURUN_*` environment variables with `NOVITA_*` fallbacks for
  credentials, domains and endpoint overrides.
- Require Go 1.25, update IDNA dependencies, and reject malformed secret host labels.

See each SDK documentation for supported operations and installation instructions.
