# hydrant

Hydrant is a CLI tool that statically renders ArgoCD ApplicationSets 
into Application manifests. By shifting from in-memory runtime evaluation to
the "rendered manifests" pattern, it enforces pure GitOps—drastically reducing
the blast radius by letting you review exact diffs before deployment. 
Deployment topology is driven by Argo CD cluster secrets, uniquely leveraging their labels
 not only for cluster targeting but as a dynamic source of ⁠the Chart values⁠.


**Input**: 
+ `testdata/cluster-init/apps/**/config-{env}.yaml`
+ `testdata/generated/clusters/{env}/*.yaml`.


**Output**: 
+ one `Application YAML` per app × cluster pair — plain pointer objects the hub applies as-is.

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
