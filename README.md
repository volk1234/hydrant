# hydrant


 Hydrant is an offline generator that reimagines the ArgoCD ApplicationSet by shifting application targeting and
 topology resolution from runtime cluster evaluation into a deterministic, static build step. It is not a replacement
 for Argo CD itself, nor is it a traditional "source hydrator" (it does not bake Helm charts into raw Kubernetes
 workloads like Deployments or Services). Instead, Hydrant replaces the dynamic ApplicationSet controller:
 it statically pairs applications with target clusters using cluster secrets, deep-merges multi-layer values files,
 substitutes environment tokens, and renders plain, auditable Argo CD Application CRDs that Argo CD applies as-is.

```
Vanilla ArgoCD:  Wrapper App ──► ApplicationSet ──► [app manifests] ──►  Applications

hydrant:  hydrant ──► [app manifests] ──► Wrapper App ──►  Applications
```

## Scope: Cluster Bootstrapping
Currently, Hydrant is primarily designed for cluster bootstrapping  — deploying foundational platform components (ingress controllers, Karpenter, cert-manager, external-secrets, observability) across multiple clusters.

## How It Works: The Two Inputs
Hydrant operates on a simple principle: Cluster Secrets (Topology) + App Configs (Intent) = Rendered Applications.

Cluster Secrets (Standard ArgoCD Secret or ExternalSecret manifests) that declare your cluster fleet. They act as the single source of truth for cluster attributes:

- Must have argocd.argoproj.io/secret-type: cluster.
- Profiles: Labels like hydrant.sh/profile-compute: "true" define what kind of workloads the cluster accepts.
- Tokens: Labels/annotations like hydrant.sh/region: eu-west-1 expose tokens (e.g. [REGION], [CLUSTER_NAME]) for templating.

App Intent-  per-environment application contract describing what to deploy and where:

- Chart source: Repo URL, chart name/path, target revision.
- Targeting: Target profile (matches clusters with that profile) and optional per-cluster overrides (namespaces, revisions).
- Values & Tokens: Layered values.yaml + optional inline_values that dynamically consume cluster tokens ([CLUSTER_NAME], [REGION]).
- ArgoCD policies: Sync waves, prune/self-heal, and cascade delete settings.

**Input example**: 
+ `testdata/cluster-init/apps/**/config-{env}.yaml`
+ `testdata/generated/clusters/{env}/*.yaml`.


**Output example**: 
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
