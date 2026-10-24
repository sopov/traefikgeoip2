.PHONY: default lint test vendor clean

default: lint test

lint:
	golangci-lint run

test:
	go test -race -count=1 -cover ./...

yaegi_test:
	yaegi test -v .	

# Re-vendor dependencies and re-apply the patches documented in patches/README.md.
vendor:
	go mod vendor
	for p in patches/*.patch; do patch -p1 < $$p; done

clean:
	go clean -testcache
