# Contributing to hydrant

Thank you for your interest in contributing to `hydrant`.

## Specification as source of truth

Behavior, inputs, outputs, and constraints (such as deterministic rendering and
zero network dependencies) are defined by the project specification. Changes to
CLI behavior or output layout should reference the relevant specification item.

## Development checks

Run the local checks before opening a pull request:

```sh
make test
make gate REPO=testdata
make compare
```

If your change is not intended to alter rendered manifests, `make compare` must
produce no diff.

## Developer Certificate of Origin (DCO)

Every commit must include a `Signed-off-by` line certifying that you have the
right to submit the work under the project license (Apache-2.0):

```sh
git commit -s -m "your commit message"
```

Pull requests without DCO sign-off on all commits will fail the `DCO` check.

## Pull requests

1. Fork the repository and create a branch from `main`.
2. Make sure all checks (`make test`, `make gate REPO=testdata`, `make compare`) pass.
3. Follow Conventional Commits for PR titles (checked in CI).
4. Fill out the pull request template.
