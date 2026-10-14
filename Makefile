-include Makefile.local

OUT  ?= /tmp/rendered
KUBECONFORM = go run github.com/yannh/kubeconform/cmd/kubeconform@v0.8.0
SCHEMA = schemas/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json
ENVS ?= dev live

.PHONY: test render check validate determinism gate compare require-repo

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