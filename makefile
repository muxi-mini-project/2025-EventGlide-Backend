DATE := $(shell date +"%m%d")

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build

run:
	nohup ./EG > eg.$(DATE).log 2>&1 &

stop:
	pkill -f EG || echo "No process found"

fmt:
	gofmt -w .

docs:
	swag init

.PHONY: build run stop fmt docs