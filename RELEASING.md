git checkout main
git pull origin main
make test
make build
```

### 2. Tag and Push

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### 3. GitHub Actions Automatically Publishes

Triggered by version tags, the workflow builds and publishes:

**Binaries (GitHub Release Assets):**
- `ingestor-linux-amd64`
- `ingestor-linux-arm64`
- `checksums.txt`

**Docker Images (GitHub Container Registry):**
- `ghcr.io/amr8t/blobsearch/ingestor:v1.0.0`
- `ghcr.io/amr8t/blobsearch/ingestor:latest`

Multi-arch: linux/amd64, linux/arm64

### 4. Create GitHub Release

Visit https://github.com/amr8t/blobsearch/releases, select the tag, and add release notes with:
- What's new
- Breaking changes
- Docker image references

## Local Building

```bash
# Build for current platform
make build

# Build for release platforms
make build-all

# Run tests
make test

# Clean
make clean
```

## Versioning

Use semantic versioning: `vX.Y.Z`

- **Major (v2.0.0)**: Breaking changes
- **Minor (v1.1.0)**: New features
- **Patch (v1.0.1)**: Bug fixes

## Manual Trigger

Run workflow manually: Actions → Release → Run workflow

## Rollback

```bash
git tag -d v1.0.0
git push origin :refs/tags/v1.0.0
```

Delete the GitHub release and optionally remove images via Packages page.

## Troubleshooting

**Workflow fails?** Check logs in Actions tab

**Images not public?** Packages → Package → Change visibility → Public

**Build failed?** Run `make test` and `make build` locally first