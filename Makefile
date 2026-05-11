.PHONY: build seed run run-http run-tcp run-udp run-grpc run-ws cli clean test deps tidy

BIN_DIR := bin

deps:
	go mod download

tidy:
	go mod tidy

build: deps
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/mangahub-server ./cmd/api-server
	go build -o $(BIN_DIR)/mangahub-tcp    ./cmd/tcp-server
	go build -o $(BIN_DIR)/mangahub-udp    ./cmd/udp-server
	go build -o $(BIN_DIR)/mangahub-grpc   ./cmd/grpc-server
	go build -o $(BIN_DIR)/mangahub-ws     ./cmd/ws-server
	go build -o $(BIN_DIR)/mangahub-seed   ./cmd/seed
	go build -o $(BIN_DIR)/mangahub-cli    ./cmd/cli

seed: build
	./$(BIN_DIR)/mangahub-seed --config config.yaml --data data/manga.json

run: build seed
	./$(BIN_DIR)/mangahub-server --config config.yaml

run-http: build
	./$(BIN_DIR)/mangahub-server --config config.yaml

run-tcp: build
	./$(BIN_DIR)/mangahub-tcp --config config.yaml

run-udp: build
	./$(BIN_DIR)/mangahub-udp --config config.yaml

run-grpc: build
	./$(BIN_DIR)/mangahub-grpc --config config.yaml

run-ws: build
	./$(BIN_DIR)/mangahub-ws --config config.yaml

cli: build
	./$(BIN_DIR)/mangahub-cli

test:
	go test ./...

clean:
	rm -rf $(BIN_DIR) data/mangahub.db
