.PHONY: build test lint fmt vet test-live clean

build:
	go build -o bin/klab ./cmd/klab

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint: vet
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

# Live boot/KVM tests. Requires an Apple-silicon M3+ host (or Linux with /dev/kvm).
# Not run by hosted CI.
test-live: build
	./test/integration/run.sh

clean:
	rm -rf bin
