# Makefile
.PHONY: proto build run test clean docker docker-build docker-run docker-compose-up docker-compose-down

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

docker: docker-build

docker-build:
	docker build -t openmachinecore:latest .

docker-run:
	docker run -p 8080:8080 -p 50051:50051 \
		-v $(PWD)/configs:/app/configs \
		openmachinecore:latest

docker-compose-up:
	docker-compose up -d

docker-compose-down:
	docker-compose down

migrate-up:
	psql postgresql://omc:omc@localhost:5432/openmachinecore < migrations/001_init.sql
