.PHONY: run test tidy fmt

run:
	go run .

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	gofmt -w main.go $$(find internal -name '*.go')