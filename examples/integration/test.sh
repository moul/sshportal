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
       moul/sshportal --debug
)
while ! nc -z localhost ${PORT}; do
    sleep 1
done
sleep 3

# integration suite
xssh() {
    set -e
    echo "+ ssh {sshportal} $@"
    ssh -q -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no localhost -p ${PORT} $@
}
# login
xssh -l invite:integration

# hostgroup/usergroup/acl
xssh -l admin hostgroup create
xssh -l admin hostgroup create --name=hg1
xssh -l admin hostgroup create --name=hg2 --comment=test
xssh -l admin usergroup inspect hg1 hg2
xssh -l admin hostgroup ls

xssh -l admin usergroup create
xssh -l admin usergroup create --name=ug1
xssh -l admin usergroup create --name=ug2 --comment=test
xssh -l admin usergroup inspect ug1 ug2
xssh -l admin usergroup ls

xssh -l admin acl create --ug=ug1 --ug=ug2 --hg=hg1 --hg=hg2 --comment=test --action=allow --weight=42
xssh -l admin acl inspect 2
xssh -l admin acl ls

# basic host create
xssh -l admin host create bob@example.org:1234
xssh -l admin host create test42
xssh -l admin host create --name=testtest --comment=test --password=test test@test.test
xssh -l admin host create --group=hg1 --group=hg2 hostwithgroups.org
xssh -l admin host inspect example test42 testtest hostwithgroups
xssh -l admin host ls

# post cleanup
#cleanup
