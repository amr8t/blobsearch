# Pre-Release Checklist

Before creating your first release, complete these steps:

## 1. GitHub Repository Setup

- [ ] Repository is public (or package permissions configured)
- [ ] Go to Settings → Actions → General
- [ ] Under "Workflow permissions", select "Read and write permissions"
- [ ] Click "Allow GitHub Actions to create and approve pull requests"
- [ ] Save

## 2. Test Locally

```bash
# Build ingestor
go build -o bin/ingestor ./cmd/ingestor

# Test harness
cd harness
make docker-up
make generate-logs type=apache count=100
make load-logs file=../generated-apache.log
make query-stats
make docker-down
cd ..
```

## 3. Test Docker Builds

```bash
# Build ingestor image
docker build -t blobsearch-ingestor-test -f Dockerfile .

# Test run
docker run --rm blobsearch-ingestor-test -h
```

## 4. Create First Release

```bash
# Ensure you're on main/master
git checkout main
git pull

# Create and push tag
git tag -a v0.1.0 -m "Initial release"
git push origin v0.1.0
```

## 5. Monitor GitHub Actions

- [ ] Go to https://github.com/amr8t/blobsearch/actions
- [ ] Watch the release workflow
- [ ] Verify both images are built successfully
- [ ] Check build logs for any errors

## 6. Make Packages Public

After the workflow completes:

- [ ] Go to https://github.com/amr8t?tab=packages
- [ ] Click on `blobsearch/ingestor`
- [ ] Click "Package settings"
- [ ] Change visibility to "Public"

## 7. Test Published Images

```bash
# Pull images
docker pull ghcr.io/amr8t/blobsearch/ingestor:v0.1.0

# Test ingestor
docker run --rm ghcr.io/amr8t/blobsearch/ingestor:v0.1.0 -h

# Test end-to-end (requires S3)
docker run -d -p 8080:8080 \
  -e ENDPOINT=https://your-s3-endpoint \
  -e ACCESS_KEY=your-key \
  -e SECRET_KEY=your-secret \
  -e BUCKET=test-bucket \
  ghcr.io/amr8t/blobsearch/ingestor:v0.1.0

echo "test log" | curl -X POST --data-binary @- http://localhost:8080/ingest
curl http://localhost:8080/stats
curl -X POST http://localhost:8080/flush
```

## 8. Create GitHub Release

- [ ] Go to https://github.com/amr8t/blobsearch/releases
- [ ] Click "Draft a new release"
- [ ] Choose tag: v0.1.0
- [ ] Release title: "BlobSearch v0.1.0"
- [ ] Add release notes (see RELEASING.md for template)
- [ ] Publish release

## 9. Update Documentation (if needed)

- [ ] Verify README.md quickstart works
- [ ] Check all links in documentation
- [ ] Ensure example code is correct

## 10. Announce

- [ ] Add link to published images in README
- [ ] Share on relevant platforms
- [ ] Update any external documentation

## Troubleshooting

### If GitHub Actions fails:

1. Check workflow logs
2. Verify Dockerfile syntax
3. Ensure go.mod/go.sum are up to date
4. Check repo permissions

### If images won't publish:

1. Verify workflow permissions (step 1)
2. Check GITHUB_TOKEN has package:write scope
3. Ensure tag matches pattern `v*`

### If images are private:

1. Follow step 6 to make them public
2. Or configure GitHub package authentication for users

## Success Criteria

✅ Workflow runs without errors
✅ Ingestor image published to ghcr.io
✅ Images can be pulled without authentication
✅ Example from README works end-to-end
✅ Documentation is accurate

---

**After completing this checklist, your first release is ready!**

See RELEASING.md for future releases.
