# build
FROM golang:1.18.2 as builder
ENV             GO111MODULE=on
WORKDIR         /go/src/moul.io/sshportal
COPY            go.mod go.sum ./
RUN             go mod download
COPY            . ./
RUN             make _docker_install

# minimal runtime
FROM            alpine
COPY            --from=builder /go/bin/sshportal /bin/sshportal
ENTRYPOINT      ["/bin/sshportal"]
CMD             ["server"]
EXPOSE          2222
HEALTHCHECK     CMD /bin/sshportal healthcheck --wait
