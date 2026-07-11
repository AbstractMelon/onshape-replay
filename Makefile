BINARY=onshape-replay
BACKEND=./backend
FRONTEND=./frontend

.PHONY: all frontend backend clean

all: frontend backend

frontend:
	cd $(FRONTEND) && pnpm install && pnpm build:prod

backend:
	cd $(BACKEND) && go build -o $(BINARY) ./cmd/server/

clean:
	rm -f $(BACKEND)/$(BINARY) $(BACKEND)/cmd/server/frontend_dist/index.html
	rm -rf $(BACKEND)/cmd/server/frontend_dist/_app
	rm -rf $(FRONTEND)/.svelte-kit $(FRONTEND)/build
