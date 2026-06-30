.PHONY: build test lint fmt vet test-live setup doctor clean

build:
	go build -o bin/klab ./cmd/klab

# Detect + auto-configure the host (installs deps, starts the accelerated VM).
setup:
	./scripts/setup.sh

# Read-only host readiness report.
doctor:
	./scripts/doctor.sh

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
