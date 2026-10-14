-include Makefile.local

OUT  ?= /tmp/rendered
KUBECONFORM = go run github.com/yannh/kubeconform/cmd/kubeconform@v0.8.0
SCHEMA = schemas/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json
ENVS ?= dev live
GOVULNCHECK = go run golang.org/x/vuln/cmd/govulncheck@v1.9.0
GOLICENSES  = go run github.com/google/go-licenses/v2@v2.0.1
NET_PKGS    = net|net/http|crypto/tls
LICENSES    = Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause

.PHONY: test render check validate determinism gate compare require-repo vulncheck nonet licenses

require-repo:
	@test -n "$(REPO)" || { echo "REPO is not set — pass REPO=/path/to/turing" >&2; exit 1; }

test:
	go vet ./...
	go test -shuffle=on ./...

render: require-repo
	 for e in $(ENVS); do go run ./cmd/hydrant -repo $(REPO) -env $$e -out $(OUT) || exit 1; done


validate: render
	$(KUBECONFORM) -strict -summary -schema-location '$(SCHEMA)' $(OUT)

determinism: require-repo
	rm -rf $(OUT)-a $(OUT)-b
	go run ./cmd/hydrant -repo $(REPO) -env dev -out $(OUT)-a
	go run ./cmd/hydrant -repo $(REPO) -env dev -out $(OUT)-b
	diff -r $(OUT)-a $(OUT)-b
	@echo "determinism: OK"

gate: require-repo
	rm -rf $(OUT)
	go run ./cmd/hydrant -repo $(REPO) -env dev -out $(OUT)
	scripts/gate-a9.sh $(REPO) dev $(OUT)

compare:
	rm -rf tmp/
	go run ./cmd/hydrant -repo testdata -env dev -out tmp/
	diff -r testdata-rendered/ tmp/
	@echo "no diff!!!"

vulncheck:
	$(GOVULNCHECK) ./...

nonet:
	@if go list -deps ./cmd/hydrant | grep -xE '$(NET_PKGS)'; then \
			echo "R12.1: network packages in the dependency graph" >&2; exit 1; fi
	@echo "nonet: OK"

licenses:
	$(GOLICENSES) check ./... --allowed_licenses=$(LICENSES)