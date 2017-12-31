# build
FROM            golang:1.9 as builder
COPY            . /go/src/github.com/moul/sshportal
WORKDIR         /go/src/github.com/moul/sshportal
RUN             make _docker_install

# minimal runtime
FROM            scratch
COPY            --from=builder /go/bin/sshportal /bin/sshportal
ENTRYPOINT      ["/bin/sshportal"]
CMD             ["server"]
