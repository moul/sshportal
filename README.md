# sshportal

[![CircleCI](https://circleci.com/gh/moul/sshportal.svg?style=svg)](https://circleci.com/gh/moul/sshportal)
[![Docker Build Status](https://img.shields.io/docker/build/moul/sshportal.svg)](https://hub.docker.com/r/moul/sshportal/)
[![Go Report Card](https://goreportcard.com/badge/github.com/moul/sshportal)](https://goreportcard.com/report/github.com/moul/sshportal)
[![GoDoc](https://godoc.org/github.com/moul/sshportal?status.svg)](https://godoc.org/github.com/moul/sshportal)
[![License](https://img.shields.io/github/license/moul/sshportal.svg)](https://github.com/moul/sshportal/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/moul/sshportal.svg)](https://github.com/moul/sshportal/releases)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fmoul%2Fsshportal.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fmoul%2Fsshportal?ref=badge_shield)

Jump host/Jump server without the jump, a.k.a Transparent SSH bastion

![sshportal demo](https://github.com/moul/sshportal/raw/master/.assets/demo.gif)

---

## Overview

![sshportal overview](https://raw.github.com/moul/sshportal/master/.assets/overview.svg?sanitize=true)

## Features

* Single autonomous binary (~10-20Mb) with no runtime dependencies (embeds ssh server and client)
* Portable / Cross-platform (regularly tested on linux and OSX/darwin)
* Store data in [Sqlite3](https://www.sqlite.org/) or [MySQL](https://www.mysql.com) (probably easy to add postgres, mssql thanks to gorm)
* Stateless -> horizontally scalable when using [MySQL](https://www.mysql.com) as the backend
* Connect to remote host using key or password
* Admin commands can be run directly or in an interactive shell
* Host management
* User management (invite, group, stats)
* Host Key management (remote host key learning)
* User Key management (multile keys per user)
* ACL management (acl+user-groups+host-groups)
* User roles (admin, trusted, standard, ...)
* User invitations (no more "give me your public ssh key please")
* Easy server installation (generate shell command to setup `authorized_keys`)
* Sensitive data encryption
* Session management (see active connections, history, stats, stop)
* Audit log (logging every user action)
* Record TTY Session
* Host Keys verifications shared across users
* Healthcheck user (replying OK to any user)
* SSH compatibility
  * ipv4 and ipv6 support
  * [`scp`](https://linux.die.net/man/1/scp) support
  * [`rsync`](https://linux.die.net/man/1/rsync) support
  * [tunneling](https://www.ssh.com/ssh/tunneling/example) (local forward, remote forward, dynamic forward) support
  * [`sftp`](https://www.ssh.com/ssh/sftp/) support
  * [`ssh-agent`](https://www.ssh.com/ssh/agent) support
  * [`X11 forwarding`](http://en.tldp.org/HOWTO/XDMCP-HOWTO/ssh.html) support
  * Git support (can be used to easily use multiple user keys on GitHub, or access your own firewalled gitlab server)
  * Do not require any SSH client modification or custom `.ssh/config`, works with every tested SSH programming libraries and every tested SSH clients
* SSH to non-SSH proxy
  * [Telnet](https://www.ssh.com/ssh/telnet) support

## (Known) limitations

* Does not work (yet?) with [`mosh`](https://mosh.org/)

## Usage

Start the server

```console
$ sshportal server
2017/11/13 10:58:35 Admin user created, use the user 'invite:BpLnfgDsc2WD8F2q' to associate a public key with this account
2017/11/13 10:58:35 SSH Server accepting connections on :2222
```

Link your SSH key with the admin account

```console
$ ssh localhost -p 2222 -l invite:BpLnfgDsc2WD8F2q
Welcome admin!

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
  ID | NAME |           URL           |   KEY   | PASS | GROUPS  | COMMENT
+----+------+-------------------------+---------+------+---------+---------+
   1 | foo  | bart@foo.example.org:22 | default |      | default |
Total: 1 hosts.
config>
```

Add the key to the server

```console
$ ssh bart@foo.example.org "$(ssh localhost -p 2222 -l admin key setup default)"
$
```

Profit

```console
ssh localhost -p 2222 -l foo
bart@foo>
```

Invite friends

*This command doesn't create a user on the remote server, it only creates an account in the sshportal database.*

```console
config> user invite bob@example.com
User 2 created.
To associate this account with a key, use the following SSH user: 'invite:NfHK5a84jjJkwzDk'.
config>
```

## Flow Diagram

![Flow Diagram](https://raw.github.com/moul/sshportal/master/.assets/flow-diagram.svg?sanitize=true)

## built-in shell

`sshportal` embeds a configuration CLI.

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
acl create [-h] [--hostgroup=HOSTGROUP...] [--usergroup=USERGROUP...] [--pattern=<value>] [--comment=<value>] [--action=<value>] [--weight=value]
acl inspect [-h] ACL...
acl ls [-h] [--latest] [--quiet]
acl rm [-h] ACL...
acl update [-h] [--comment=<value>] [--action=<value>] [--weight=<value>] [--assign-hostgroup=HOSTGROUP...] [--unassign-hostgroup=HOSTGROUP...] [--assign-usergroup=USERGROUP...] [--unassign-usergroup=USERGROUP...] ACL...

# config management
config help
config backup [-h] [--indent] [--decrypt]
config restore [-h] [--confirm] [--decrypt]

# event management
event help
event ls [-h] [--latest] [--quiet]
event inspect [-h] EVENT...

# host management
host help
host create [-h] [--name=<value>] [--password=<value>] [--comment=<value>] [--key=KEY] [--group=HOSTGROUP...] [--hop=HOST] <username>[:<password>]@<host>[:<port>]
host inspect [-h] [--decrypt] HOST...
host ls [-h] [--latest] [--quiet]
host rm [-h] HOST...
host update [-h] [--name=<value>] [--comment=<value>] [--key=KEY] [--assign-group=HOSTGROUP...] [--unassign-group=HOSTGROUP...] [--set-hop=HOST] [--unset-hop] HOST...

# hostgroup management
hostgroup help
hostgroup create [-h] [--name=<value>] [--comment=<value>]
hostgroup inspect [-h] HOSTGROUP...
hostgroup ls [-h] [--latest] [--quiet]
hostgroup rm [-h] HOSTGROUP...

# key management
key help
key create [-h] [--name=<value>] [--type=<value>] [--length=<value>] [--comment=<value>]
key inspect [-h] [--decrypt] KEY...
key ls [-h] [--latest] [--quiet]
key rm [-h] KEY...
key setup [-h] KEY
key show [-h] KEY

# session management
session help
session ls [-h] [--latest] [--quiet]
session inspect [-h] SESSION...

# user management
user help
user invite [-h] [--name=<value>] [--comment=<value>] [--group=USERGROUP...] <email>
user inspect [-h] USER...
user ls [-h] [--latest] [--quiet]
user rm [-h] USER...
user update [-h] [--name=<value>] [--email=<value>] [--set-admin] [--unset-admin] [--assign-group=USERGROUP...] [--unassign-group=USERGROUP...] USER...

# usergroup management
usergroup help
hostgroup create [-h] [--name=<value>] [--comment=<value>]
usergroup inspect [-h] USERGROUP...
usergroup ls [-h] [--latest] [--quiet]
usergroup rm [-h] USERGROUP...

# other
exit [-h]
help, h
info [-h]
version [-h]
```

## Docker

Docker is the recommended way to run sshportal.

An [automated build is setup on the Docker Hub](https://hub.docker.com/r/moul/sshportal/tags/).

```console
# Start a server in background
#   mount `pwd` to persist the sqlite database file
docker run -p 2222:2222 -d --name=sshportal -v "$(pwd):$(pwd)" -w "$(pwd)" moul/sshportal:v1.7.1

# check logs (mandatory on first run to get the administrator invite token)
docker logs -f sshportal
```

The easier way to upgrade sshportal is to do the following:

```sh
# we consider you were using an old version and you want to use the new version v1.7.1

# stop and rename the last working container + backup the database
docker stop sshportal
docker rename sshportal sshportal_old
cp sshportal.db sshportal.db.bkp

# run the new version
docker run -p 2222:2222 -d --name=sshportal -v "$(pwd):$(pwd)" -w "$(pwd)" moul/sshportal:v1.7.1
# check the logs for migration or cross-version incompabitility errors
docker logs -f sshportal
```

Now you can test ssh-ing to sshportal to check if everything looks OK.

In case of problem, you can rollback to the latest working version with the latest working backup, using:

```sh
docker stop sshportal
docker rm sshportal
cp sshportal.db.bkp sshportal.db
docker rename sshportal_old sshportal
docker start sshportal
docker logs -f sshportal
```

## Manual Install

Get the latest version using GO.

```sh
go get -u github.com/moul/sshportal
```

## portal alias (.ssh/config)

Edit your `~/.ssh/config` file (create it first if needed)

```ini
Host portal
  User      admin
  Port      2222       # portal port
  HostName  127.0.0.1  # portal hostname
```

```bash
# you can now run a shell using this:
ssh portal
# instead of this:
ssh localhost -p 2222 -l admin

# or connect to hosts using this:
ssh hostname@portal
# instead of this:
ssh localhost -p 2222 -l hostname
```

## Backup / Restore

sshportal embeds built-in backup/restore methods which basically import/export JSON objects:

```sh
# Backup
ssh portal config backup  > sshportal.bkp

# Restore
ssh portal config restore < sshportal.bkp
```

This method is particularly useful as it should be resistant against future DB schema changes (expected during development phase).

I suggest you to be careful during this development phase, and use an additional backup method, for example:

```sh
# sqlite dump
sqlite3 sshportal.db .dump > sshportal.sql.bkp

# or just the immortal cp
cp sshportal.db sshportal.db.bkp
```

## Demo data

The following servers are freely available, without external registration,
it makes it easier to quickly test `sshportal` without configuring your own servers to accept sshportal connections.

```
ssh portal host create new@sdf.org
ssh sdf@portal

ssh portal host create test@whoami.filippo.io
ssh whoami@portal

ssh portal host create test@chat.shazow.net
ssh chat@portal
```

## Healthcheck

By default, `sshportal` will return `OK` to anyone sshing using the `healthcheck` user without checking for authentication.

```console
$ ssh healthcheck@sshportal
OK
$
```

the `healtcheck` user can be changed using the `healthcheck-user` option.

---

Alternatively, you can run the built-in healthcheck helper (requiring no ssh client nor ssh key):

Usage: `sshportal healthcheck [--addr=host:port] [--wait] [--quiet]

```console
$ sshportal healthcheck --addr=localhost:2222; echo $?
$ 0
```

---

Wait for sshportal to be healthy, then connect

```console
$ sshportal healthcheck --wait && ssh sshportal -l admin
config>
```

## Scaling

`sshportal` is stateless but relies on a database to store configuration and logs.

By default, `sshportal` uses a local [sqlite](https://www.sqlite.org/) database which isn't scalable by design.

You can run multiple instances of `sshportal` sharing a same [MySQL](https://www.mysql.com) database, using `sshportal --db-conn=user:pass@host/dbname?parseTime=true --db-driver=mysql`.

![sshportal cluster with MySQL backend](https://raw.github.com/moul/sshportal/master/.assets/cluster-mysql.svg?sanitize=true)

See [examples/mysql](http://github.com/moul/sshportal/tree/master/examples/mysql).

## Under the hood

* Docker first (used in dev, tests, by the CI and in production)
* Backed by (see [dep graph](https://godoc.org/github.com/moul/sshportal?import-graph&hide=2)):
  * SSH
    * https://github.com/gliderlabs/ssh: SSH server made easy (well-designed golang library to build SSH servers)
    * https://godoc.org/golang.org/x/crypto/ssh: both client and server SSH protocol and helpers
  * Database
    * https://github.com/jinzhu/gorm/: SQL orm
    * https://github.com/go-gormigrate/gormigrate: Database migration system
  * Built-in shell
    * https://github.com/olekukonko/tablewriter: Ascii tables
    * https://github.com/asaskevich/govalidator: Valide user inputs
    * https://github.com/dustin/go-humanize: Human-friendly representation of technical data (time ago, bytes, ...)
    * https://github.com/mgutz/ansi: Terminal color helpers
    * https://github.com/urfave/cli: CLI flag parsing with subcommands support

![sshportal data model](https://raw.github.com/moul/sshportal/master/.assets/sql-schema.svg?sanitize=true)

## Note

This is totally experimental for now, so please file issues to let me know what you think about it!


## License
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fmoul%2Fsshportal.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fmoul%2Fsshportal?ref=badge_large)
