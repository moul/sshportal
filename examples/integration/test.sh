#!/bin/sh -e
# Setup a new sshportal and performs some checks

PORT=${PORT:-2222}
SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN=integration

# pre cleanup
cleanup() {
    docker rm -f -v sshportal-integration 2>/dev/null >/dev/null || true
}
cleanup

# start server
( set -xe;
  docker run \
       -d \
       -e SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN=${SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN} \
       --name=sshportal-integration \
       -p${PORT}:2222 \
       moul/sshportal
)
sleep 3 # FIXME: replace with port checker

# integration suite
xssh() {
    set -e
    echo "+ ssh {sshportal} $@"
    ssh -q -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no localhost -p ${PORT} $@
}
xssh -l invite:integration
xssh -l admin host create bob@example.org:1234
xssh -l admin host inspect example
xssh -l admin host create test42
xssh -l admin host inspect test42
xssh -l admin host create --name=testtest --comment=test --password=test test@test.test
xssh -l admin host inspect testtest
xssh -l admin host ls

# post cleanup
#cleanup
