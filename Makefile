-include Makefile.local

ifndef REPO
$(error REPO is not set — pass REPO=/path/to/turing)
endif
OUT  ?= /tmp/rendered
KUBECONFORM = go run github.com/yannh/kubeconform/cmd/kubeconform@v0.7.0
SCHEMA = testdata/schemas/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json

.PHONY: test render check validate

test:
	go vet ./cmd
	go test ./cmd

render:
	go run ./cmd -repo $(REPO) -env dev -out $(OUT)
	go run ./cmd -repo $(REPO) -env live -out $(OUT)


validate: render
	$(KUBECONFORM) -strict -summary -schema-location '$(SCHEMA)' $(OUT)
