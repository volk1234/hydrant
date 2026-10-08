-include Makefile.local

ifndef REPO
$(error REPO is not set — pass REPO=/path/to/turing)
endif
OUT  ?= /tmp/rendered
KUBECONFORM = go run github.com/yannh/kubeconform/cmd/kubeconform@v0.7.0
SCHEMA = testdata/schemas/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json

.PHONY: test render check validate determinism gate

test:
	go vet ./cmd
	go test ./cmd

render:
	go run ./cmd -repo $(REPO) -env dev -out $(OUT)
	go run ./cmd -repo $(REPO) -env live -out $(OUT)


validate: render
	$(KUBECONFORM) -strict -summary -schema-location '$(SCHEMA)' $(OUT)

determinism:
	rm -rf $(OUT)-a $(OUT)-b
	go run ./cmd -repo $(REPO) -env dev -out $(OUT)-a
	go run ./cmd -repo $(REPO) -env dev -out $(OUT)-b
	diff -r $(OUT)-a $(OUT)-b
	@echo "determinism: OK"

gate:
	rm -rf $(OUT)
	go run ./cmd -repo $(REPO) -env dev -out $(OUT)
	scripts/gate-a9.sh $(REPO) dev $(OUT)

compare:
	rm -rf tmp/
	go run ./cmd -repo testdata -env dev -out tmp/
	diff -r testdata-rendered/ tmp/
	@echo "no diff!!!"