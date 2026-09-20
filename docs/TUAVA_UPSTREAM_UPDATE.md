# Tuava upstream update workflow

This fork keeps the official repository as `upstream` and stores Tuava changes as a small commit stack on top of the official branch.

## Branch model

- `upstream/main`: latest official Sub2API.
- `main`: deployable Tuava build (`upstream/main` plus Tuava commits).
- `archive/*`: immutable rollback points created before each upgrade.
- `upgrade/*`: optional staging worktrees for testing a new official release.

Do not copy a new official source tree over this repository and do not squash official and custom code into one snapshot commit. Both destroy Git's three-way merge context.

## Upgrade

### GitHub automation

`.github/workflows/upstream-sync.yml` checks the latest official release every day. It performs a three-way merge on a dedicated branch, runs the full backend suite and frontend production build, then opens a PR. If Git reports conflicts, the workflow leaves `main` untouched and opens/updates an Issue containing the conflicted files.

The workflow can also be started manually from **Actions → Upstream Sync → Run workflow**, following either the latest official release, official `main`, or an explicit tag/ref.

Always merge the generated sync PR with a **merge commit**. Squashing or rebasing it discards official ancestry and makes later updates harder.

### Local fallback

From a clean Tuava branch:

```bash
./tools/update-upstream.sh upstream/main
```

Or pin a release:

```bash
./tools/update-upstream.sh v0.2.7
```

The script fetches official refs, creates an archive branch, and rebases only the Tuava commit stack. Git `rerere` is enabled so previously resolved conflicts can be reused.

## Verification

```bash
cd backend
go test ./...

cd ../frontend
npm exec --yes --package=pnpm@9.15.9 -- pnpm install --frozen-lockfile
npm exec --yes --package=pnpm@9.15.9 -- pnpm run build
```

Then build a local image and test against the existing data volumes before moving production:

```bash
cd ../deploy
docker compose --env-file .env -f docker-compose.dev.yml build sub2api
docker compose --env-file .env -f docker-compose.dev.yml up -d sub2api
curl -f http://127.0.0.1:18080/health
```

## Conflict rules

1. Keep official schema/API behavior unless a Tuava requirement explicitly changes it.
2. Reapply ticket persistence through the dedicated ticket files and typed settings fields; never replace whole official service files.
3. Keep account proxy pools in `accounts.extra.proxy_pool_ids` until an official schema provides an equivalent first-class relation.
4. Resolve UI conflicts at component/field granularity; do not choose an entire side for large account or settings components.
5. Run focused tests after each custom commit and the full suites before deployment.
