.PHONY: test test-verbose


test:
	go test ./...

test-verbose:
	go test ./... -v