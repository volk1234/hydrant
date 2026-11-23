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
Currently, Hydrant is primarily designed for cluster bootstrapping — deploying foundational platform components (ingress controllers, Karpenter, cert-manager, external-secrets, observability) across multiple clusters.

## How It Works: The Two Inputs
Hydrant operates on a simple principle: Cluster Secrets (Topology) + App Configs (Intent) = Rendered Applications.

Cluster Secrets (standard Argo CD `Secret` or `ExternalSecret` manifests) declare your cluster fleet and act as the single source of truth for cluster attributes:

- Must have `argocd.argoproj.io/secret-type: cluster`.
- **Profiles:** Labels like `hydrant.sh/profile-compute: "true"` define what kind of workloads the cluster accepts.
- **Tokens:** Labels/annotations like `hydrant.sh/region: eu-west-1` expose tokens (e.g. `[REGION]`, `[CLUSTER_NAME]`) for templating.

App Intent — per-environment application contract describing what to deploy and where:

- **Chart source:** Repo URL, chart name/path, target revision.
- **Targeting:** Target profile (matches clusters with that profile) and optional per-cluster overrides (namespaces, revisions).
- **Values & Tokens:** Layered `values.yaml` + optional `inline_values` that dynamically consume cluster tokens (`[CLUSTER_NAME]`, `[REGION]`).
- **Argo CD policies:** Sync waves, prune/self-heal, and cascade delete settings.

### Quick Example

Repository config ([`testdata/hydrant.yaml`](testdata/hydrant.yaml)) sets the label prefix and directory layout:

```yaml
labelPrefix: hydrant.sh/
apps: cluster-init/apps
clusters: generated/clusters
```

Cluster Secret ([`cluster-c-1.yaml`](testdata/generated/clusters/dev/cluster-c-1.yaml)) + App Config ([`karpenter/config-dev.yaml`](testdata/cluster-init/apps/karpenter/config-dev.yaml)):

```yaml
# Cluster Secret (c1)                     # App config (karpenter/config-dev.yaml)
labels:                                   destination:
  hydrant.sh/profile-base: "true"           profile: "base"
  hydrant.sh/region_short: "euw1"         inline_values: |
stringData:                                 clusterName: [CLUSTER_NAME]
  name: c1                                  region: "[REGION_SHORT]"
```

Run:

```sh
hydrant -repo testdata -env dev -out /tmp/rendered
```

Rendered output ([`testdata-rendered/dev/c1/karpenter.yaml`](testdata-rendered/dev/c1/karpenter.yaml)):

```yaml
kind: Application
metadata:
  name: karpenter-c1
spec:
  source:
    helm:
      valuesObject:
        clusterName: c1
        region: "euw1"
```

## Usage

```sh
hydrant -repo <checkout> -env dev -out rendered/dev   # render
hydrant -repo . -env live -out rendered/live -partition-label hydrant.sh/rendered: "true" # scoped render
```

## Properties

- **Deterministic & offline** — pure function of the checkout: no network,
  no kube/Argo API, no Vault, no secret material. Argo CD Vault Plugin (AVP)
  placeholders resolve at runtime on the hub repo-server.
- **Fast** — full dev+live render in ~90 ms.
- **Minimal** — stdlib + `go.yaml.in/yaml/v3`; targeting logic is a single
  unit-tested function.

## Development

```sh
make test                                # go vet + unit & golden tests
go test -run TestGolden -update          # regenerate goldens after intended changes
make compare                             # verify testdata output matches testdata-rendered/
make nonet                               # verify zero network packages in binary
make validate                            # kubeconform against the vendored schema
```

## Installation

### From source

```sh
go install github.com/volk1234/hydrant/cmd/hydrant@v0.2.0
```

### From release archive (with verification)

Download the archive and checksums, then verify the signature, checksum, and build provenance before running the binary:

```sh
gh release download v0.2.0 -R volk1234/hydrant \
  -p 'hydrant_0.2.0_linux_amd64.tar.gz' \
  -p 'checksums.txt*'

# Verify checksums signature (cosign keyless via Sigstore)
cosign verify-blob \
  --certificate-identity-regexp '^https://github\.com/volk1234/hydrant/\.github/workflows/release\.yaml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --bundle checksums.txt.sigstore.json checksums.txt

# Verify archive checksum
sha256sum --check --ignore-missing checksums.txt

# Verify SLSA build provenance
gh attestation verify hydrant_0.2.0_linux_amd64.tar.gz -R volk1234/hydrant

tar xzf hydrant_0.2.0_linux_amd64.tar.gz hydrant
./hydrant -version
```

## Compatibility

`hydrant` follows [Semantic Versioning](https://semver.org/). The following are considered breaking changes:

| Change | Example | Breaking |
|---|---|---|
| `hydrant.yaml` format | Key renamed or made required | Yes |
| Application config format | Key renamed or default value changed | Yes |
| Output layout | Different file path or object name | Yes |
| Output content for identical input | Changed field in rendered output | Yes |
| New optional input key | New key that preserves prior behavior when omitted | No |
| CLI flags | Flag removed or semantics changed | Yes |

Before `v1.0.0`, breaking changes increment the minor version (`0.x.0`). From `v1.0.0` onward, breaking changes increment the major version (`x.0.0`). Every breaking change is documented in the release notes.

## No Network Access

`hydrant` makes **zero network calls** — it never connects to the internet, Kubernetes API, Argo CD API, or secret stores.

This is enforced in CI on every pull request by `make nonet`, which fails if `net`, `net/http`, or `crypto/tls` appears anywhere in the binary's dependency graph. Air-gapped and restricted-network environments can run `hydrant` without network policy exceptions.

## Hydrant vs. Argo CD Source Hydrator

`hydrant` replaces `ApplicationSet` generation: given cluster configs/labels and app configs, it produces `Application` manifests. **Argo CD Source Hydrator** renders the sources of a single `Application` into raw manifests and commits them to Git. The two tools solve different problems and can be used together.
