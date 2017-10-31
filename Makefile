.PHONY: install
install:
	go install .

.PHONY: dev
dev:
	-go get github.com/githubnemo/CompileDaemon
	CompileDaemon -build="make install" -command="sshportal --demo --debug" .
