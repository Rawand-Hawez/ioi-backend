.PHONY: dev build up down status logs reset db-gen db-setup db-seed db-reset db-branch-create db-branch-switch db-branch-drop test test-e2e test-verbose up-prod down-prod

dev:
	go run cmd/api/main.go

build:
	go build -o bin/api cmd/api/main.go

db-gen:
	sqlc generate

# Run after first `docker compose up` — creates profiles, app tables, triggers, and seeds dev user.
# GoTrue must be running (it creates auth.users on first boot).
POSTGREST_PASSWORD ?= $(shell grep ^POSTGREST_PASSWORD= .env | cut -d '=' -f2)

db-setup:
	@echo "Running post-GoTrue setup (profiles, app tables, triggers)..."
	@docker exec -i skeleton_postgres psql -U postgres -d app_database < db/setup/post_gotrue.sql
	@echo "Creating business structure tables..."
	@docker exec -i skeleton_postgres psql -U postgres -d app_database < db/setup/business-structure.sql
	@echo "Creating inventory tables..."
	@docker exec -i skeleton_postgres psql -U postgres -d app_database < db/setup/inventory.sql
	@echo "Creating party tables..."
	@docker exec -i skeleton_postgres psql -U postgres -d app_database < db/setup/party.sql
	@echo "Syncing PostgREST authenticator password..."
	@docker exec -i skeleton_postgres psql -U postgres -d app_database -c "ALTER ROLE authenticator WITH PASSWORD '$(POSTGREST_PASSWORD)';"
	@echo "Seeding development user..."
	@docker exec -i skeleton_postgres psql -U postgres -d app_database < db/setup/seed.sql
	@echo "Setup complete."

db-seed:
	@docker exec -i skeleton_postgres psql -U postgres -d app_database < db/setup/seed.sql

# --- LOCAL DATABASE BRANCHING ---
# Extract active DB and establish the protected master database
MAIN_DB := app_database
ACTIVE_DB ?= $(shell grep ^POSTGRES_DB= .env | cut -d '=' -f2)

db-branch-create:
	@if [ -z "$(BRANCH)" ]; then echo "Error: BRANCH name required (e.g., make db-branch-create BRANCH=feature_x)"; exit 1; fi
	@echo "Terminating active connections on '$(ACTIVE_DB)'..."
	@docker exec -i skeleton_postgres psql -U postgres -d postgres -c "SELECT pg_terminate_backend(pg_stat_activity.pid) FROM pg_stat_activity WHERE pg_stat_activity.datname = '$(ACTIVE_DB)' AND pid <> pg_backend_pid();" > /dev/null
	@echo "Cloning database from '$(ACTIVE_DB)' to '$(BRANCH)'..."
	@docker exec -i skeleton_postgres psql -U postgres -d postgres -c "CREATE DATABASE $(BRANCH) WITH TEMPLATE $(ACTIVE_DB);"
	@echo "✅ Branch '$(BRANCH)' created successfully."

db-branch-switch:
	@if [ -z "$(BRANCH)" ]; then echo "Error: BRANCH name required. Use make db-branch-switch BRANCH=$(MAIN_DB) to revert."; exit 1; fi
	@echo "Switching .env database target to '$(BRANCH)'..."
	@sed -i.bak "s/^POSTGRES_DB=.*/POSTGRES_DB=$(BRANCH)/" .env && rm -f .env.bak
	@echo "Restarting specific containers to map new .env configurations..."
	@docker compose up -d --force-recreate postgrest gotrue
	@echo "✅ Switched architecture to branch: $(BRANCH)"

db-branch-drop:
	@if [ -z "$(BRANCH)" ]; then echo "Error: BRANCH name required"; exit 1; fi
	@if [ "$(BRANCH)" = "$(MAIN_DB)" ]; then echo "🚨 CRITICAL ERROR: Refusing to drop the protected master database ($(MAIN_DB))."; exit 1; fi
	@if [ "$(BRANCH)" = "$(ACTIVE_DB)" ]; then echo "🚨 ERROR: Cannot drop the currently active branch. Switch off it first."; exit 1; fi
	@echo "Dropping database branch '$(BRANCH)'..."
	@docker exec -i skeleton_postgres psql -U postgres -d postgres -c "DROP DATABASE IF EXISTS $(BRANCH);"
	@echo "✅ Branch '$(BRANCH)' dropped permanently."

up:
	docker compose up -d

down:
	docker compose down

status:
	docker compose ps

logs:
	docker compose logs -f

reset:
	docker compose down -v
	docker compose up -d

db-reset:
	@echo "Dropping and recreating database..."
	@docker exec -i skeleton_postgres psql -U postgres -c "DROP DATABASE IF EXISTS app_database;"
	@docker exec -i skeleton_postgres psql -U postgres -c "CREATE DATABASE app_database;"
	@$(MAKE) db-setup
	@echo "Database reset complete."

up-prod:
	docker compose -f docker-compose.prod.yml up -d --build

down-prod:
	docker compose -f docker-compose.prod.yml down

test:
	@set -a && . ./.env && set +a && go test -v ./tests/...

test-verbose:
	@set -a && . ./.env && set +a && go test -v -count=1 ./tests/...

test-e2e:
	@echo "Running End-to-End Architectural Tests..."
	@set -a && . ./.env && set +a && go test -v ./tests/e2e_test.go

