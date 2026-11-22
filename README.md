# Gpurun Sandbox SDKs

Official Gpurun Sandbox SDKs for Go, Java, TypeScript and Python. Every SDK keeps
the existing sandbox API available for compatibility and adds the NovitaBox
`boxapi` control plane plus the `boxd` data plane.

## SDKs

| Language | Package |
| --- | --- |
| [Go](sdk-go/README.md) | `github.com/gpurun/gpurun-sandbox/sdk-go` |
| [Java](sdk-java/README.md) | `ai.gpurun:gpurun-sandbox` |
| TypeScript | `gpurun-sandbox` |
| Python | `gpurun-sandbox` (import as `gpurun_sandbox`) |

Set `GPURUN_API_KEY` before creating a client; `NOVITA_API_KEY` and the other
`NOVITA_*` variables remain supported as fallbacks. The boxapi client defaults to
`http://localhost:8080` and the boxd client to `http://localhost:8082`; both can
be overridden through the SDK options or `GPURUN_BASE_URL` / `GPURUN_DATA_URL`.

The source repository is [gpurun/gpurun-sandbox](https://github.com/gpurun/gpurun-sandbox).

## Development and releases

See [CONTRIBUTING.md](CONTRIBUTING.md), [RELEASING.md](RELEASING.md),
[CHANGELOG.md](CHANGELOG.md) and [package-consumer tests](release-smoke-tests/README.md).
Report vulnerabilities using [SECURITY.md](SECURITY.md).

## License

MIT — see [LICENSE](LICENSE). Both native SDKs also include their license in the
component directory; Java binary/source archives carry META-INF/LICENSE; Javadoc archives include doc-files/LICENSE.
