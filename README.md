# hydrant

Hydrates dry ArgoCD ApplicationSet configs into rendered Argo CD `Application` manifests — the [rendered manifests pattern](https://akuity.io/blog/the-rendered-manifests-pattern), replacing runtime ApplicationSet evaluation.
Tool make the topology using cluster secrets - define what goes where with what values.

**Input**: `testdata/cluster-init/apps/**/config-{env}.yaml`
+ `testdata/generated/clusters/{env}/*.yaml`.
**Output**: one `Application YAML` per app × cluster pair — plain pointer objects the hub applies as-is.

## Usage

```sh
hydrant -repo <checkout> -env dev -out rendered/dev   # render
hydrant -repo . -env live -out rendered/live -partition-label hydrant.sh/rendered: "true" # scoped render
```

## Properties

- **Deterministic & offline** — pure function of the checkout: no network,
  no kube/Argo API, no Vault, no secret material. AVP placeholders resolve
  at runtime on the hub repo-server.
- **Fast** — full dev+live render in ~90 ms.
- **Minimal** — stdlib + `gopkg.in/yaml.v3`; targeting logic is one
  unit-tested function (`resolve.go`).

## Development

```sh
go test ./...                            # unit + golden tests
go test -run TestGolden -update          # regenerate goldens after intended changes
make validate                            # kubeconform against the vendored schema
make update-schema ARGOCD_VERSION=vX.Y.Z # refresh schema on hub Argo CD upgrades
```
