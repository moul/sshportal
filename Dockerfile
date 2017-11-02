# build
FROM            golang:1.9 as builder
COPY            . /go/src/github.com/moul/sshportal
WORKDIR         /go/src/github.com/moul/sshportal
RUN             CGO_ENABLED=1 go build -tags netgo -ldflags '-extldflags "-static"' -v -o /go/bin/sshportal

# minimal runtime
FROM            scratch
COPY            --from=builder /go/bin/sshportal /bin/sshportal
ENTRYPOINT      ["/bin/sshportal"]
