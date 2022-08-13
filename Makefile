GOENV=CGO_ENABLED=0 GOFLAGS="-count=1"
GOCMD=$(GOENV) go
GOTEST=$(GOCMD) test -covermode=atomic -coverprofile=./coverage.out -timeout=20m

dev:
	air

tidy:
	$(GOCMD) mod tidy
