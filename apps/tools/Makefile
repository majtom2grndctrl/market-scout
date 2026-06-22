# Build the Go tools into bin/. Run from apps/tools/ (same CWD the binaries
# expect for godotenv's .env.local lookup). bin/ is gitignored.
#
# Adding a command? Add its directory name to CMDS.
#
# Targets always shell out to `go build` and let Go's build cache decide what
# to recompile — it tracks transitive source deps correctly, which a Makefile
# prerequisite list does not. A no-op `go build` is near-instant.

CMDS := fetcher migrate onboard batch-enrich strip-boilerplate mcp

.PHONY: build vet check clean $(CMDS)

build: $(CMDS)

$(CMDS):
	go build -o bin/$@ ./cmd/$@

vet:
	go vet ./...

# Pre-commit gate: whole tree compiles, vet passes, then build the binaries.
check:
	go build ./...
	go vet ./...
	$(MAKE) build

clean:
	rm -f $(addprefix bin/,$(CMDS))
