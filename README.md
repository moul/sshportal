# sshportal

Jump host/Jump server without the jump, a.k.a Transparent SSH bastion

```
                       ┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
                                  DMZ           │
┌────────┐             │             ┌────────┐
│ homer  │───▶╔═════════════════╗───▶│ host1  │ │
└────────┘    ║                 ║    └────────┘
┌────────┐    ║                 ║    ┌────────┐ │
│  bart  │───▶║    sshportal    ║───▶│ host2  │
└────────┘    ║                 ║    └────────┘ │
┌────────┐    ║                 ║    ┌────────┐
│  lisa  │───▶╚═════════════════╝───▶│ host3  │ │
└────────┘             │             └────────┘
┌────────┐                           ┌────────┐ │
│  ...   │             │             │  ...   │
└────────┘                           └────────┘ │
                       └ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
```

## Features

* Host management
* User management
* User Group management
* Host Group management
* Host Key management
* User Key management
* ACL management
* Connect to host using key or password
* Admin commands can be run directly or in an interactive shell

## Usage

Start the server

```console
$ sshportal
2017/11/13 10:58:35 Admin user created, use the user 'invite:BpLnfgDsc2WD8F2q' to associate a public key with this account
2017/11/13 10:58:35 SSH Server accepting connections on :2222
```

Link your SSH key with the admin account

```console
$ ssh localhost -p 2222 -l invite:BpLnfgDsc2WD8F2q
Welcome Administrator!

Your key is now associated with the user "admin@sshportal".
Shared connection to localhost closed.
$
```

Drop an interactive administrator shell

```console
ssh localhost -p 2222 -l admin


    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/


config>
```

Create your first host

```console
config> host create bart@foo.example.org
1
config>
```

List hosts

```console
config> host ls
  ID | NAME |           URL           |   KEY   | PASS | GROUPS | COMMENT
+----+------+-------------------------+---------+------+--------+---------+
   1 | foo  | bart@foo.example.org:22 | default |      |      1 |
Total: 1 hosts.
config>
```

Get the default key in authorized_keys format

```console
config> key inspect default
[...]
    "PubKey": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCvUP/8FedyIe+a+RWU4KvJ1+iZwtWmY9czJubLwN4RcjKHQMzLqWC7pKZHAABCZjLJjVD/3Zb53jZwbh7mysAkocundMpvUL5+Yb4a8lDiflXkdXT9fZCx+ibJBk4jRnKLGIneSzVtFEerEwQKKnKQoCgPkZwCDaL/jHhDlOmAvxqAJrjiy42HXwppX2UuF8zujs6OKHRYJ/Q1vo0caa6/o1eoyXE9OrOwIk+IcAN3YIQi/B1BOlZOQBzHIZz83AFlD2TcPhyYcbxPyKGih84Zr3rQaaP1WiaiPqxzp3s5OhTLthc5XtCSLzmRSLvgC2eFdNhBDB5KLtO2khBkz5ID",
[...]
config>
```

Add this key to the server

```console
$ ssh bart@foo.example.org
> umask 077; mkdir -p .ssh; echo ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCvUP/8FedyIe+a+RWU4KvJ1+iZwtWmY9czJubLwN4RcjKHQMzLqWC7pKZHAABCZjLJjVD/3Zb53jZwbh7mysAkocundMpvUL5+Yb4a8lDiflXkdXT9fZCx+ibJBk4jRnKLGIneSzVtFEerEwQKKnKQoCgPkZwCDaL/jHhDlOmAvxqAJrjiy42HXwppX2UuF8zujs6OKHRYJ/Q1vo0caa6/o1eoyXE9OrOwIk+IcAN3YIQi/B1BOlZOQBzHIZz83AFlD2TcPhyYcbxPyKGih84Zr3rQaaP1WiaiPqxzp3s5OhTLthc5XtCSLzmRSLvgC2eFdNhBDB5KLtO2khBkz5ID >> .ssh/authorized_keys
```

Profit

```console
ssh localhost -p 2222 -l foo
bart@foo>
```

Invite friends

```console
config> user invite bob@example.com
User 2 created.
To associate this account with a key, use the following SSH user: 'invite-NfHK5a84jjJkwzDk'.
config>
```

## Install

Get the latest version using GO (recommended way):

```sh
go get -u github.com/moul/sshportal
```
