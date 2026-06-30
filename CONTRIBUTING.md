# Contributing to klab

Thanks for your interest. klab is early — issues, design feedback, and PRs are welcome.

## Development setup

Prerequisites:
- **Go 1.22+** (`brew install go`) — the `klab` CLI/orchestrator.
- For live tests only: an **Apple silicon M3+ Mac on macOS 15+** with
  [lima](https://lima-vm.io) (`brew install lima`), or any Linux host with `/dev/kvm`.

```sh
make build      # build ./bin/klab
make test       # unit + contract tests (no KVM needed)
make lint       # go vet + gofmt check
make test-live  # live boot/KVM tests — requires an M3+ host (not run in CI by default)
```

## Test policy

Every change ships with tests. We split them deliberately (see `../PLAN.md` §7):
- **Unit/contract/lint** run on hosted CI with no special hardware. Most logic
  (validation, hashing, topology graph, driver selection, generated command lines) is
  pure and lives here — keep it that way: generate command lines as *data we can
  snapshot* before exec.
- **Live integration** (real boot, `/dev/kvm`, in-guest asserts) needs an M3+ host. It
  runs nightly / on a self-hosted runner and is a release gate, not a per-PR gate.

Rules:
- New CLI subcommand → at least one happy-path test and one bad-input error test.
- New driver → must pass the shared driver conformance suite.
- Don't silently retry flaky tests; quarantine with a tracking issue.
- Don't weaken a golden snapshot/hash without explaining why in the PR.

## Style
- Go: `gofmt`-clean, `go vet`-clean. Keep `internal/` logic pure and testable.
- Shell (in-guest scripts): `shellcheck`-clean, `set -euo pipefail`.
- Commits: imperative subject; explain *why* in the body. Sign off if you can
  (`git commit -s`).

## Scope
klab is a research **lab**, not a production cluster manager. See the non-goals in
`docs/architecture.md` before proposing large features.

## License
By contributing, you agree that your contributions are licensed under the project's
[Apache License 2.0](LICENSE).
