# Makefile
.PHONY: proto build run test clean docker-up docker-down

PROTOC_INCLUDES = -I. \
	-I./third_party/googleapis \
	-I/usr/include

proto:
	protoc $(PROTOC_INCLUDES) \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
		api/proto/*.proto

build:
	go build -o bin/openmachinecore cmd/server/main.go

run:
	go run cmd/server/main.go

test:
	go test ./...

clean:
	rm -rf bin/

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

migrate-up:
	psql postgresql://omc:omc@localhost:5432/openmachinecore < migrations/001_init.sql
