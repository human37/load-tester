.PHONY: build clean deps run help

help:
	@echo "Available targets:"
	@echo "  build              - Build the load test binary"
	@echo "  deps               - Install dependencies"
	@echo "  clean              - Clean build artifacts"
	@echo "  run req=<file> env=<env> - Run load test with specified config file and environment"
	@echo ""
	@echo "Usage:"
	@echo "  make run req=mutations/your-file.yaml env=dev"

deps:
	go mod tidy

build: deps
	go build -o .load-tester .

clean:
	rm -f .load-tester
	rm -rf results/*
	rm -rf logs/*

run: build
	@if [ -z "$(req)" ]; then \
		echo "Error: Please specify a config file."; \
		echo "Usage: make run req=mutations/your-file.yaml env=dev"; \
		echo ""; \
		echo "Available configs:"; \
		find mutations -name "*.yaml" -type f 2>/dev/null | head -10 || echo "  (no mutations directory found)"; \
		exit 1; \
	fi
	@if [ -z "$(env)" ]; then \
		echo "Error: Please specify an environment."; \
		echo "Usage: make run req=mutations/your-file.yaml env=dev"; \
		exit 1; \
	fi
	./.load-tester -config $(req) -env $(env)

%:
	@:
