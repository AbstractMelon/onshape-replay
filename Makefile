BINARY=onshape-replay
BACKEND=./backend
FRONTEND=./frontend
FRONTEND_OUT=$(BACKEND)/cmd/server/frontend_dist

.PHONY: all frontend backend clean

all: frontend backend

frontend:
	cd $(FRONTEND) && pnpm install && pnpm build:prod
	# Go's //go:embed ignores files/dirs starting with "_" or ".",
	# so rename _app/ -> app/ and rewrite references in HTML.
	if [ -d $(FRONTEND_OUT)/_app ]; then \
		mv $(FRONTEND_OUT)/_app $(FRONTEND_OUT)/app; \
		find $(FRONTEND_OUT) -name '*.html' -exec sed -i 's|/_app/|/app/|g' {} +; \
	fi

backend:
	cd $(BACKEND) && go build -o $(BINARY) ./cmd/server/

clean:
	rm -f $(BACKEND)/$(BINARY)
	rm -f $(FRONTEND_OUT)/index.html
	rm -rf $(FRONTEND_OUT)/app
	rm -rf $(FRONTEND)/.svelte-kit $(FRONTEND)/build
