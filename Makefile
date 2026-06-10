.PHONY: clean build test

clean:
	go clean
	rm -f $(GOPATH)/bin/*

build:
	npm --prefix ui install
	npm --prefix ui run build
	go install ./...

test:
	go test ./... -count=1
	go vet ./...
