# Releasing

OpenOutreach uses **semver tags on `main`**.

## Branch model

| Branch | Role |
|--------|------|
| `main` | Stable, releasable |
| `dev` (optional) | Integration / staging before cut |

Production tags always point at `main`.

## Cut a release

1. Ensure CI is green on `main`.
2. Update [CHANGELOG.md](../CHANGELOG.md) under `## Unreleased` → version section.
3. Tag and push:

```bash
git checkout main
git pull
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

4. GitHub Actions [`.github/workflows/release.yml`](../.github/workflows/release.yml) builds:
   - `cold-cli` and `outreachd` binaries (linux/darwin amd64+arm64)
   - checksums
   - GitHub Release with notes from the tag annotation / CHANGELOG

5. Optionally publish the Container image used by Wrangler from the same tag (Dockerfile at repo root).

## Versioning guidance

- **MAJOR** — breaking API/MCP/schema changes operators must migrate for
- **MINOR** — new endpoints, tools, dashboard features, backward compatible
- **PATCH** — fixes, docs, dependency bumps

Pre-1.0 (`v0.x`): MINOR may include breaking changes; call them out in the changelog.

## Do not

- Release from dirty trees or feature branches
- Ship images with baked secrets
- Tag without running `go test` + `web` build locally if CI is skipped
