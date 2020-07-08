GOPKG ?= moul.io/sshportal
GOBINS ?= .
DOCKER_IMAGE ?= moul/sshportal

VERSION ?= `git describe --tags --always`
VCS_REF ?= `git rev-parse --short HEAD`
GO_INSTALL_OPTS = -ldflags="-X main.GitSha=$(VCS_REF) -X main.GitTag=$(VERSION)"
PORT ?= 2222

include rules.mk

DB_VERSION ?=	v$(shell grep -E 'ID: "[0-9]+",' pkg/bastion/dbinit.go | tail -n 1 | cut -d'"' -f2)
AES_KEY ?=	my-dummy-aes-key

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

.PHONY: backup
backup:
	mkdir -p data/backups
	cp sshportal.db data/backups/$(shell date +%s)-$(DB_VERSION)-sshportal.sqlite

doc:
	dot -Tsvg ./.assets/overview.dot > ./.assets/overview.svg
	dot -Tsvg ./.assets/cluster-mysql.dot > ./.assets/cluster-mysql.svg
	dot -Tsvg ./.assets/flow-diagram.dot > ./.assets/flow-diagram.svg
	dot -Tpng ./.assets/overview.dot > ./.assets/overview.png
	dot -Tpng ./.assets/cluster-mysql.dot > ./.assets/cluster-mysql.png
	dot -Tpng ./.assets/flow-diagram.dot > ./.assets/flow-diagram.png

.PHONY: goreleaser
goreleaser:
	GORELEASER_GITHUB_TOKEN=$(GORELEASER_GITHUB_TOKEN) GITHUB_TOKEN=$(GITHUB_TOKEN) goreleaser --rm-dist

.PHONY: goreleaser-dry-run
goreleaser-dry-run:
	goreleaser --snapshot --skip-publish --rm-dist
