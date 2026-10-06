TEST?=$$(go list ./... | grep -v 'vendor')

default: install

build:
	go build -o terraform-provider-vergeio .

install:
	go install .

# Local release archives. A published release is the v* tag workflow
# (.github/workflows/release.yml), which runs goreleaser release --clean.
release:
	goreleaser release --snapshot --clean --skip=sign

lint:
	golangci-lint run

generate:
	cd tools && go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. --provider-name vergeio

test:
	go test $(TEST) -v $(TESTARGS) -timeout=30s -parallel=4

testacc:
	TF_ACC=1 go test $(TEST) -v $(TESTARGS) -timeout 120m -parallel=4

sweep:
	TF_ACC=1 TF_ACC_SWEEP=1 go test ./internal/acctest -run TestSweep -count=1 -v -timeout 30m

testunit:
	go test $(TEST) -v $(TESTARGS) -timeout=30s -run='Test[^Acc]'

testrace:
	go test $(TEST) -v $(TESTARGS) -timeout=60s -race

testclean:
	go clean -testcache

.PHONY: default build install release lint generate test testacc sweep testunit testrace testclean
