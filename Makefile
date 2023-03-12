GO_MILTER_DIR := $(shell go list -f '{{.Dir}}' github.com/d--j/go-milter)

integration:
	docker build -q --progress=plain -t go-milter-integration "$(GO_MILTER_DIR)/integration/docker" && \
	docker run --rm -w /usr/src/root/integration -v $(PWD):/usr/src/root go-milter-integration \
	go run github.com/d--j/go-milter/integration/runner -filter '.*' ./tests

.PHONY: integration
