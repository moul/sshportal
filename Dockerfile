# build
FROM golang:1.12.1 as builder
ENV             GO111MODULE=on
COPY            . /go/src/moul.io/sshportal
WORKDIR         /go/src/moul.io/sshportal
RUN             make _docker_install

# minimal runtime
FROM            alpine
COPY            --from=builder /go/bin/sshportal /bin/sshportal
ENTRYPOINT      ["/bin/sshportal"]
CMD             ["server"]
EXPOSE          2222
HEALTHCHECK     CMD /bin/sshportal healthcheck --wait
