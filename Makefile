.PHONY: check build test lint build-frontend build-backend notices dev run

# One verification gate — run at the end of every wave cycle.
check:
	@bash scripts/check.sh

# Backend
build-backend:
	cd backend && go build ./...

test:
	cd backend && go test ./...

# Frontend (web/ owns the whole Vite/React project — see web/package.json)
lint:
	cd web && npm run lint

build-frontend:
	cd web && npm run build

# Regenerate THIRD-PARTY-NOTICES.txt (root) + site/licenses.txt from the real
# dependency graph (Go modules + npm + vendored site assets). Served at
# /licenses.txt. Re-run after changing go.mod, web/package.json, or site vendor files.
notices:
	./scripts/gen-notices.sh

# Full single-binary build (frontend embedded)
build:
	cd web && npm run build:all

# Dev loop
dev:
	cd web && npm run dev

run:
	cd backend && go run ./cmd/wede
