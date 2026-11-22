# Contributing

Use Go 1.25+, JDK 11+, Maven 3.9+, Node 24+ and Python 3.9+. Start a feature or
fix branch from main and open a pull request with a description of the behavior
change and its validation.

```sh
cd sdk-go
go build ./...
go vet ./...
go test -race ./...
cd ../sdk-java
mvn test
cd ../sdk-ts
npm ci && npm run build && node --test test/*.mjs
cd ../sdk-python
python3 -m compileall -q gpurun_sandbox
python3 -m unittest discover -s tests -v
```

Run `python3 -m unittest discover -s .github/scripts/tests -v` and
`python3 release-smoke-tests/local_packages.py` from the root to validate release
infrastructure and installable packages. These checks use local fixtures, need
no API credentials and create no cloud resources. See
[consumer testing](release-smoke-tests/README.md) for isolated-cache and opt-in
live testing options.

Keep request names, response parsing, timeouts and errors aligned across all four
SDKs. Wire changes to `boxapi` and `boxd` must preserve the bare-JSON success
format, the Bearer credential header, the boxd sandbox-ID header and the 5-byte
Connect stream framing. Public examples should begin with the `Gpurun` facade.
Add regression tests for behavior changes and update CHANGELOG.md and relevant
README sections.

Tests and examples must create their own resources and clean them up. Never use
hard-coded customer resource IDs, commit credentials, or print authentication
headers. Report security issues using [SECURITY.md](SECURITY.md).

Contributions are licensed under the [MIT License](LICENSE). Releases follow
[RELEASING.md](RELEASING.md); contributors do not need publishing credentials.
