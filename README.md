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

## CLI

sshportal embeds a configuration CLI.

By default, the configuration user is `admin`, (can be changed using `--config-user=<value>` when starting the server.

Each commands can be run directly by using this syntax: `ssh admin@portal.example.org <command> [args]`:

```
ssh admin@portal.example.org host inspect toto
```

You can enter in interactive mode using this syntax: `ssh admin@portal.example.org`


### Synopsis

```sh
# acl management
acl help
acl create [-h] [--hostgroup=<value>...] [--usergroup=<value>...] [--pattern=<value>] [--comment=<value>] [--action=<value>] [--weight=value]
acl inspect [-h] <id> [<id> [<id>...]]
acl ls [-h]
acl rm [-h] <id> [<id> [<id>...]]

# host management
host help
host create [-h] [--name=<value>] [--password=<value>] [--fingerprint=<value>] [--comment=<value>] [--key=<value>] [--group=<value>] <user>[:<password>]@<host>[:<port>]
host inspect [-h] <id or name> [<id or name> [<id or name>...]]
host ls [-h]
host rm [-h] <id or name> [<id or name> [<id or name>...]]

# hostgroup management
hostgroup help
hostgroup create [-h] [--name=<value>] [--comment=<value>]
hostgroup inspect [-h] <id or name> [<id or name> [<id or name>...]]
hostgroup ls [-h]
hostgroup rm [-h] <id or name> [<id or name> [<id or name>...]]

# key management
key help
key create [-h] [--name=<value>] [--type=<value>] [--length=<value>] [--comment=<value>]
key inspect [-h] <id or name> [<id or name> [<id or name>...]]
key ls [-h]
key rm [-h] <id or name> [<id or name> [<id or name>...]]

# user management
user help
user invite [-h] [--name=<value>] [--comment=<value>] [--group=<value>] <email>
user inspect [-h] <id or email> [<id or email> [<id or email>...]]
user ls [-h]
user rm [-h] <id or email> [<id or email> [<id or email>...]]

# usergroup management
usergroup help
hostgroup create [-h] [--name=<value>] [--comment=<value>]
usergroup inspect [-h] <id or name> [<id or name> [<id or name>...]]
usergroup ls [-h]
usergroup rm [-h] <id or name> [<id or name> [<id or name>...]]

# other
help, h
info [-h]
version [-h]
```

## Install

Get the latest version using GO (recommended way):

```sh
go get -u github.com/moul/sshportal
```
