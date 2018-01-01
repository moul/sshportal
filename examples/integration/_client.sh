#!/bin/sh -e

mkdir -p ~/.ssh
cp /integration/client_test_rsa ~/.ssh/id_rsa
chmod -R 700 ~/.ssh
cat >~/.ssh/config <<EOF
Host sshportal
    UserKnownHostsFile /dev/null
    StrictHostKeyChecking no
    Port 2222
    HostName sshportal
EOF

#ping -c 1 sshportal
sleep 3

set -x

# login
ssh sshportal -l invite:integration

# hostgroup/usergroup/acl
ssh sshportal -l admin hostgroup create
ssh sshportal -l admin hostgroup create --name=hg1
ssh sshportal -l admin hostgroup create --name=hg2 --comment=test
ssh sshportal -l admin usergroup inspect hg1 hg2
ssh sshportal -l admin hostgroup ls

ssh sshportal -l admin usergroup create
ssh sshportal -l admin usergroup create --name=ug1
ssh sshportal -l admin usergroup create --name=ug2 --comment=test
ssh sshportal -l admin usergroup inspect ug1 ug2
ssh sshportal -l admin usergroup ls

ssh sshportal -l admin acl create --ug=ug1 --ug=ug2 --hg=hg1 --hg=hg2 --comment=test --action=allow --weight=42
ssh sshportal -l admin acl inspect 2
ssh sshportal -l admin acl ls

# basic host create
ssh sshportal -l admin host create bob@example.org:1234
ssh sshportal -l admin host create test42
ssh sshportal -l admin host create --name=testtest --comment=test --password=test test@test.test
ssh sshportal -l admin host create --group=hg1 --group=hg2 hostwithgroups.org
ssh sshportal -l admin host inspect example test42 testtest hostwithgroups
ssh sshportal -l admin host ls

# backup/restore
ssh sshportal -l admin config backup --indent --ignore-events > backup-1
ssh sshportal -l admin config restore --confirm < backup-1
ssh sshportal -l admin config backup --indent --ignore-events  > backup-2
(
    cat backup-1 | grep -v '"date":' | grep -v 'tedAt":' > backup-1.clean
    cat backup-2 | grep -v '"date":' | grep -v 'tedAt":' > backup-2.clean
    set -xe
    diff backup-1.clean backup-2.clean
)
