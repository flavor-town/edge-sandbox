include .env
.PHONY: test test-verbose run-script

test:
	go test ./...

test-verbose:
	go test ./... -v

run-script:
	forge script script/EdgeSetup.s.sol:EdgeSetup --sig "deployNativeToken()" --legacy --rpc-url $(EDGE_URL) -vvvv --private-key $(PRIVATE_KEY)

run-rootchain:
	forge script script/EdgeSetup.s.sol:EdgeSetup --sig "mintMoreTokens()" --rpc-url $(ROOTCHAIN_URL) -vvvv --private-key $(PRIVATE_KEY) 