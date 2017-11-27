GIT_SHA ?=	$(shell git rev-parse HEAD)
GIT_TAG ?=	$(shell git describe --tags --always)
GIT_BRANCH ?=	$(shell git rev-parse --abbrev-ref HEAD)
LDFLAGS ?=	-X main.GIT_SHA=$(GIT_SHA) -X main.GIT_TAG=$(GIT_TAG) -X main.GIT_BRANCH=$(GIT_BRANCH)
VERSION ?=	$(shell grep 'VERSION =' main.go | cut -d'"' -f2)
PORT ?=		2222
AES_KEY ?=	my-dummy-aes-key

.PHONY: install
install:
	go install -ldflags '$(LDFLAGS)' .

.PHONY: docker.build
docker.build:
	docker build -t moul/sshportal .

.PHONY: integration
integration:
	PORT="$(PORT)" bash ./examples/integration/test.sh

.PHONY: _docker_install
_docker_install:
	CGO_ENABLED=1 go build -ldflags '-extldflags "-static" $(LDFLAGS)' -tags netgo -v -o /go/bin/sshportal

.PHONY: dev
dev:
	-go get github.com/githubnemo/CompileDaemon
	CompileDaemon -exclude-dir=.git -exclude=".#*" -color=true -command="./sshportal --debug --bind-address=:$(PORT) --aes-key=$(AES_KEY)" .

.PHONY: test
test:
	go test -i .
	go test -v .

.PHONY: backup
backup:
	mkdir -p data/backups
	cp sshportal.db data/backups/$(shell date +%s)-$(VERSION)-sshportal.sqlite
