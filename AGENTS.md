# Agent notes

This is a Flynn **plugin** repo (`flynn-plugin.json`). Copy it for `kind: app` or `kind: resource-provider` plugins. Go 1.24, `GOFLAGS=-mod=mod`. Images are Linux/amd64 squashfs layered on Flynn’s published ubuntu-noble (GitHub `images.json.gz`).

## Tests are required

Do not land behavior without tests in the **same change**.

- New or changed Go logic: `*_test.go` next to the code (`go test ./...`).
- `cmd/plugin-build` (manifest, Flynn base selection, layer verify): unit tests in `cmd/plugin-build/*_test.go`.
- Install hooks and scripts: if they grow real logic, add a test or a grep-guard so the path cannot silently disappear.
- Run `./script/run-unit-tests` (native on Linux; Docker on macOS). `gofmt -s` must be clean. Unit tests write HTML coverage under `coverage/` (gitignored).

Skip tests only when the change cannot regress (typo in comments, LICENSE). Say so in the commit body.

## Keep docs in sync

Do not land behavior without docs in the **same change**.

- `README.md`: install/update/uninstall commands, usage, the commands table, injected env vars, ports/TLS, layout, and the release flow must match `flynn-plugin.json` and the code. Commands use the canonical `noun:verb` form; the space form is only an alias.
- `AGENTS.md`: update when the test layout, plugin contract, or agent-facing workflow changes.
- `flynn-plugin.json` is documentation too: `cli.doc`, `cli.usage`, `setup` prompts, and `resource_missing` text are what users see. Keep them accurate and in `noun:verb` form.
- The Flynn repo mirrors this plugin in `docs/content/plugins.md` (and `docs/content/databases/<engine>.md` for resource providers). When behavior visible to Flynn users changes, make the matching change in `randy-girard/flynn` (sibling `../flynn` checkout) or note it in the commit body / PR.

Skip docs only when the change cannot be seen by a user or operator (internal refactor, test-only). Say so in the commit body. See `.cursor/rules/docs-sync.mdc`.

## Semantic git commits

Use [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(optional-scope): <imperative summary>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `ci`, `build`, `chore`.

- **Always commit logically**: one concern per commit. Do not dump an entire session or mixed features into one catch-all commit.
- Do not mix a feature with an unrelated cleanup, rename, or formatting pass.
- Subject is why it matters, not a file list. No trailing period required; keep it to one line.
- Put tests in the same commit as the behavior they cover (`feat`/`fix` with tests), not a later `test:` dump unless the commit is tests-only.
- Stage whole files by path. Do not use `git add -p` or `git add -i`. If one file mixes concerns, still commit the other files separately.
- Do not commit `dist/`, `.plugin-build-cache/`, or `script/docker/dev/.image-built`.

Examples:

```text
feat: overlay plugin packages on Flynn ubuntu-noble
fix: verify Flynn layers by sha512_256, not build-input id
test: skip busybox blobstore when picking ubuntu-noble
docs: document FLYNN_VERSION pinning for plugin-build
ci: pass GITHUB_TOKEN when resolving the Flynn release
```

Commit when asked. Push only when asked.

## Plugin contract

- Do not rebuild Ubuntu from a cloud image; `plugin-build` must pull Flynn’s ubuntu-noble layer.
- Pin Flynn with `build.base.version` / `-flynn-version` for published releases; `latest` is for local builds.
- Import Flynn APIs as `github.com/randy-girard/flynn/...`. `go.mod` must `require github.com/randy-girard/flynn`. Do not vendor Flynn and do not `replace` it with a sibling `../flynn`.
- Do not put datastore packages in a shared Flynn base. This template’s image is the example app only.
- `flynn-host plugin:install` is operator-only; the user `flynn` CLI does not install plugins.
