# Generate gRPC stubs and OpenAPI from api/proto.
.PHONY: generate
generate:
	@mkdir -p internal/proto api/swagger
	protoc \
		--proto_path=api/proto \
		--proto_path=third_party/googleapis \
		--proto_path=third_party \
		--go_out=internal/proto --go_opt=paths=source_relative \
		--go_opt=default_api_level=API_OPAQUE \
		--go-grpc_out=internal/proto --go-grpc_opt=paths=source_relative \
		--openapiv2_out=api/swagger \
		--openapiv2_opt=allow_merge=true,merge_file_name=gophkeeper,json_names_for_fields=false \
		api/proto/*.proto
	go run ./scripts/swagger2yaml.go -in api/swagger/gophkeeper.swagger.json -out api/swagger.yaml

.PHONY: test
test:
	go test ./... -cover -race

.PHONY: cover
cover:
	bash scripts/check-coverage.sh

.PHONY: build
build:
	mkdir -p bin
	go build -o bin/gophkeeper-server ./cmd/server
	go build -ldflags "-X main.version=dev -X main.buildDate=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		-o bin/gophkeeper-client ./cmd/client

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: db-up
db-up:
	docker compose -f deployments/docker-compose.yml up -d

.PHONY: db-down
db-down:
	docker compose -f deployments/docker-compose.yml down
