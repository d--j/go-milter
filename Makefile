GO_MILTER_INTEGRATION_DIR := $(shell cd integration && go list -f '{{.Dir}}' github.com/d--j/go-milter/integration)

integration:
	docker build -q --progress=plain -t go-milter-integration "$(GO_MILTER_INTEGRATION_DIR)/docker" && \
	docker run --rm --hostname=mx.example.com -w /usr/src/root/integration -v $(PWD):/usr/src/root go-milter-integration \
	go run github.com/d--j/go-milter/integration/runner -filter '.*' -mtaFilter '.*' ./tests

.PHONY: integration
