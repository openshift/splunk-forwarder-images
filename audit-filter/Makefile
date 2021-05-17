GOENV=CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on
GOFLAGS=-ldflags='-extldflags=-static' -trimpath

build: audit-log-exporter

audit-log-exporter: *.go pkg/*/*.go vendor
	$(GOENV) go build $(GOFLAGS) -o $@

vendor: go.sum
	$(GOENV) go mod vendor

go.sum: go.mod
	$(GOENV) go mod tidy
	touch go.mod go.sum

clean:
	rm -f audit-log-exporter go.sum

distclean:
	git clean -ffdx

