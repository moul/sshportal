PACKAGE ?=	github.com/moul/sshportal
GIT_SHA ?=	$(shell git rev-parse HEAD)
GIT_TAG ?=	$(shell git describe --tags --always)
GIT_BRANCH ?=	$(shell git rev-parse --abbrev-ref HEAD)
LDFLAGS ?=	"-X $(PACKAGE)/main.GIT_SHA=$(GIT_SHA) -X $(PACKAGE)/main.GIT_TAG=$(GIT_TAG) -X $(PACKAGE)/main.GIT_BRANCH=$(GIT_BRANCH)"

.PHONY: install
install:
	go install -ldflags $(LDFLAGS) .

.PHONY: dev
dev:
	-go get github.com/githubnemo/CompileDaemon
	CompileDaemon -exclude-dir=.git -exclude=".#*" -color=true -command="./sshportal --demo --debug" .

.PHONY: test
test:
	go test -i .
	go test -v .
