.PHONY: default lint test vendor yaegi clean

default: lint test

lint:
	golangci-lint run

test:
	go test -race -count=1 -cover ./...

# Re-vendor dependencies and re-apply the patches documented in patches/README.md.
vendor:
	go mod vendor
	for p in patches/*.patch; do patch -p1 < $$p; done

# Load the plugin with the Yaegi interpreter the way Traefik does (see yaegitest/).
yaegi:
	cd yaegitest && go run . ..

clean:
	go clean -testcache
