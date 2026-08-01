.PHONY: all build run test proto clean docker-up docker-down run-web

all: proto build

proto:
	@echo "Generating Go and Python code from proto..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/diagnosis.proto
	python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. proto/diagnosis.proto

build:
	@echo "Building Go services..."
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/mock-infra ./cmd/mock-infra

run-gateway:
	go run ./cmd/gateway

run-mock:
	go run ./cmd/mock-infra

run-brain:
	cd brain && uv run python server.py

run-web:
	cd web && npm run dev

dev:
	@echo "Starting all services in development mode..."
	@echo "Run in separate terminals: make run-gateway, make run-mock, make run-brain, make run-web"

test:
	go test -v ./...

docker-up:
	docker-compose -f deploy/docker-compose.yml up --build -d

docker-down:
	docker-compose -f deploy/docker-compose.yml down

clean:
	rm -rf bin/
	find . -name "*_pb2.py" -delete
	find . -name "*_pb2_grpc.py" -delete
	find . -name "*.pb.go" -delete
