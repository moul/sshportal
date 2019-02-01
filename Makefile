GIT_SHA ?=	$(shell git rev-parse HEAD)
GIT_TAG ?=	$(shell git describe --tags --always)
GIT_BRANCH ?=	$(shell git rev-parse --abbrev-ref HEAD)
LDFLAGS ?=	-X main.GitSha=$(GIT_SHA) -X main.GitTag=$(GIT_TAG) -X main.GitBranch=$(GIT_BRANCH)
VERSION ?=	$(shell grep 'VERSION =' main.go | cut -d'"' -f2)
AES_KEY ?=	my-dummy-aes-key
GO ?=		GO111MODULE=on go

.PHONY: install
install:
	$(GO) install -v -ldflags '$(LDFLAGS)' .

.PHONY: docker.build
docker.build:
	docker build -t moul/sshportal .

.PHONY: integration
integration:
	cd ./examples/integration && make

.PHONY: _docker_install
_docker_install:
	CGO_ENABLED=1 $(GO) build -ldflags '-extldflags "-static" $(LDFLAGS)' -tags netgo -v -o /go/bin/sshportal

.PHONY: dev
dev:
	-$(GO) get github.com/githubnemo/CompileDaemon
	CompileDaemon -exclude-dir=.git -exclude=".#*" -color=true -command="./sshportal server --debug --bind-address=:$(PORT) --aes-key=$(AES_KEY) $(EXTRA_RUN_OPTS)" .

.PHONY: test
test:
	$(GO) test -i ./...
	$(GO) test -v ./...

.PHONY: lint
lint:
	golangci-lint run --verbose ./...

.PHONY: backup
backup:
	mkdir -p data/backups
	cp sshportal.db data/backups/$(shell date +%s)-$(VERSION)-sshportal.sqlite

doc:
	dot -Tsvg ./.assets/overview.dot > ./.assets/overview.svg
	dot -Tsvg ./.assets/cluster-mysql.dot > ./.assets/cluster-mysql.svg
	dot -Tsvg ./.assets/flow-diagram.dot > ./.assets/flow-diagram.svg
	dot -Tpng ./.assets/overview.dot > ./.assets/overview.png
	dot -Tpng ./.assets/cluster-mysql.dot > ./.assets/cluster-mysql.png
	dot -Tpng ./.assets/flow-diagram.dot > ./.assets/flow-diagram.png
