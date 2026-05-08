FRONTEND_DIR := frontend
COMPOSE := docker compose

INTEGRATION_POSTGRES_URL := jdbc:postgresql://localhost:5432/ragent
INTEGRATION_POSTGRES_USER := postgres
INTEGRATION_POSTGRES_PASSWORD := postgres
INTEGRATION_RUSTFS_URL := http://localhost:9000
INTEGRATION_RUSTFS_ACCESS_KEY_ID := rustfsadmin
INTEGRATION_RUSTFS_SECRET_ACCESS_KEY := rustfsadmin
INTEGRATION_RUSTFS_BUCKET := knowledge

.PHONY: test test-go test-frontend lint lint-frontend build build-frontend integration-up integration-down test-integration test-integration-pipeline test-integration-upload

test: test-go

test-go:
	go test ./... -count=1

test-frontend:
	cd $(FRONTEND_DIR) && npm run build

lint: lint-frontend

lint-frontend:
	cd $(FRONTEND_DIR) && npm run lint

build: build-frontend

build-frontend:
	cd $(FRONTEND_DIR) && npm run build

integration-up:
	$(COMPOSE) up -d postgres object-storage object-storage-init

integration-down:
	$(COMPOSE) down

test-integration: test-integration-pipeline test-integration-upload

test-integration-pipeline:
	RAG_INTEGRATION_PIPELINE=1 \
	POSTGRES_URL=$(INTEGRATION_POSTGRES_URL) \
	POSTGRES_USER=$(INTEGRATION_POSTGRES_USER) \
	POSTGRES_PASSWORD=$(INTEGRATION_POSTGRES_PASSWORD) \
	go test ./internal/app/knowledge/service/test -run 'TestKnowledgeDocumentPipelineIntegration|TestPipelineFetcherNotFound|TestPipelineIndexerMissingDependency' -count=1

test-integration-upload:
	RAG_INTEGRATION_UPLOAD=1 \
	RAG_INTEGRATION_RUSTFS=1 \
	POSTGRES_URL=$(INTEGRATION_POSTGRES_URL) \
	POSTGRES_USER=$(INTEGRATION_POSTGRES_USER) \
	POSTGRES_PASSWORD=$(INTEGRATION_POSTGRES_PASSWORD) \
	RUSTFS_URL=$(INTEGRATION_RUSTFS_URL) \
	RUSTFS_ACCESS_KEY_ID=$(INTEGRATION_RUSTFS_ACCESS_KEY_ID) \
	RUSTFS_SECRET_ACCESS_KEY=$(INTEGRATION_RUSTFS_SECRET_ACCESS_KEY) \
	RUSTFS_BUCKET=$(INTEGRATION_RUSTFS_BUCKET) \
	go test ./internal/app/knowledge/service/test ./internal/adapter/storage/s3/test -run 'TestKnowledgeDocumentUploadIntegration|TestFileStorageUploadOpenDeleteIntegration' -count=1
