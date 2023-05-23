# sshportal

[![CircleCI](https://circleci.com/gh/moul/sshportal.svg?style=svg)](https://circleci.com/gh/moul/sshportal)
[![Go Report Card](https://goreportcard.com/badge/moul.io/sshportal)](https://goreportcard.com/report/moul.io/sshportal)
[![GoDoc](https://godoc.org/moul.io/sshportal?status.svg)](https://godoc.org/moul.io/sshportal)
[![Financial Contributors on Open Collective](https://opencollective.com/sshportal/all/badge.svg?label=financial+contributors)](https://opencollective.com/sshportal) [![License](https://img.shields.io/github/license/moul/sshportal.svg)](https://github.com/moul/sshportal/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/moul/sshportal.svg)](https://github.com/moul/sshportal/releases)
<!-- temporarily broken? [![Docker Build Status](https://img.shields.io/docker/build/moul/sshportal.svg)](https://hub.docker.com/r/moul/sshportal/) -->

Jump host/Jump server without the jump, a.k.a Transparent SSH bastion

<img src="https://raw.githubusercontent.com/moul/sshportal/master/.assets/bastion.jpg" width="50%">

Features include: independence of users and hosts, convenient user invite system, connecting to servers that don't support SSH keys, various levels of access, and many more. Easy to install, run and configure.

![Flow Diagram](https://raw.githubusercontent.com/moul/sshportal/master/.assets/flow-diagram.png)

---

## Contents

<!-- toc -->

- [Installation and usage](#installation-and-usage)
- [Use cases](#use-cases)
- [Features and limitations](#features-and-limitations)
- [Docker](#docker)
- [Manual Install](#manual-install)
- [Backup / Restore](#backup--restore)
- [built-in shell](#built-in-shell)
- [Demo data](#demo-data)
- [Shell commands](#shell-commands)
- [Healthcheck](#healthcheck)
- [portal alias (.ssh/config)](#portal-alias-sshconfig)
- [Scaling](#scaling)
- [Under the hood](#under-the-hood)
- [Testing](#testing)

<!-- tocstop -->

---

## Installation and usage

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

If the association fails and you are prompted for a password, verify that the host you're connecting from has a SSH key set up or generate one with ```ssh-keygen -t rsa```

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

Demo gif:
![sshportal demo](https://github.com/moul/sshportal/raw/master/.assets/demo.gif)

---

## Use cases

Used by educators to provide temporary access to students. [Feedback from a teacher](https://github.com/moul/sshportal/issues/64). The author is using it in one of his projects, *pathwar*, to dynamically configure hosts and users, so that he can give temporary accesses for educational purposes.

*vptech*, the vente-privee.com technical team (a group of over 6000 people) is using it internally to manage access to servers/routers, saving hours on configuration management and not having to share the configuration information.

There are companies who use a jump host to monitor connections at a single point.

A hosting company is using SSHportal for its “logging” feature, among others. As every session is logged and introspectable, they have a detailed history of who performed which action. This company made its own contribution to the project, allowing the support of [more than 65.000 sessions in the database](https://github.com/moul/sshportal/pull/76).

The project has also received [multiple contributions from a security researcher](https://github.com/moul/sshportal/pulls?q=is%3Apr+author%3Asabban+sort%3Aupdated-desc) that made a thesis on quantum cryptography. This person uses SSHportal in their security-hardened hosting company.

If you need to invite multiple people to an event (hackathon, course, etc), the day before the event you can create multiple accounts at once, print the invite, and distribute the paper.

---

## Features and limitations

* Single autonomous binary (~10-20Mb) with no runtime dependencies (embeds ssh server and client)
* Portable / Cross-platform (regularly tested on linux and OSX/darwin)
* Store data in [Sqlite3](https://www.sqlite.org/) or [MySQL](https://www.mysql.com) (probably easy to add postgres, mssql thanks to gorm)
* Stateless -> horizontally scalable when using [MySQL](https://www.mysql.com) as the backend
* Connect to remote host using key or password
* Admin commands can be run directly or in an interactive shell
* Host management
* User management (invite, group, stats)
* Host Key management (create, remove, update, import)
* Automatic remote host key learning
* User Key management (multiple keys per user)
* ACL management (acl+user-groups+host-groups)
* User roles (admin, trusted, standard, ...)
* User invitations (no more "give me your public ssh key please")
* Easy server installation (generate shell command to setup `authorized_keys`)
* Sensitive data encryption
* Session management (see active connections, history, stats, stop)
* Audit log (logging every user action)
* Record TTY Session (with [ttyrec](https://en.wikipedia.org/wiki/Ttyrec) format, use `ttyplay` for replay)
* Tunnels logging
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

**(Known) limitations**

* Does not work (yet?) with [`mosh`](https://mosh.org/)
* It is not possible for a user to access a host with the same name as the user. This is easily circumvented by changing the user name, especially since the most common use cases does not expose it.
* It is not possible to access a host named `healthcheck` as this is a built-in command.

---

## Docker

Docker is the recommended way to run sshportal.

An [automated build is setup on the Docker Hub](https://hub.docker.com/r/moul/sshportal/tags/).

```console
# Start a server in background
#   mount `pwd` to persist the sqlite database file
docker run -p 2222:2222 -d --name=sshportal -v "$(pwd):$(pwd)" -w "$(pwd)" moul/sshportal:v1.10.0

# check logs (mandatory on first run to get the administrator invite token)
docker logs -f sshportal
```

The easier way to upgrade sshportal is to do the following:

```sh
# we consider you were using an old version and you want to use the new version v1.10.0

# stop and rename the last working container + backup the database
docker stop sshportal
docker rename sshportal sshportal_old
cp sshportal.db sshportal.db.bkp

# run the new version
docker run -p 2222:2222 -d --name=sshportal -v "$(pwd):$(pwd)" -w "$(pwd)" moul/sshportal:v1.10.0
# check the logs for migration or cross-version incompatibility errors
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

---

## Manual Install

Get the latest version using GO.

```sh
GO111MODULE=on go get -u moul.io/sshportal
```

---

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

---

## built-in shell

`sshportal` embeds a configuration CLI.

By default, the configuration user is `admin`, (can be changed using `--config-user=<value>` when starting the server. The shell is also accessible through `ssh [username]@portal.example.org`.

Each command can be run directly by using this syntax: `ssh admin@portal.example.org <command> [args]`:

```
ssh admin@portal.example.org host inspect toto
```

You can enter in interactive mode using this syntax: `ssh admin@portal.example.org`

![sshportal overview](https://raw.github.com/moul/sshportal/master/.assets/overview.png)

---

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

---

## Shell commands

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
host create [-h] [--name=<value>] [--password=<value>] [--comment=<value>] [--key=KEY] [--group=HOSTGROUP...] [--hop=HOST] [--logging=MODE] <username>[:<password>]@<host>[:<port>]
host inspect [-h] [--decrypt] HOST...
host ls [-h] [--latest] [--quiet]
host rm [-h] HOST...
host update [-h] [--name=<value>] [--comment=<value>] [--key=KEY] [--assign-group=HOSTGROUP...] [--unassign-group=HOSTGROUP...] [--logging-MODE] [--set-hop=HOST] [--unset-hop] HOST...

# hostgroup management
hostgroup help
hostgroup create [-h] [--name=<value>] [--comment=<value>]
hostgroup inspect [-h] HOSTGROUP...
hostgroup ls [-h] [--latest] [--quiet]
hostgroup rm [-h] HOSTGROUP...

# key management
key help
key create [-h] [--name=<value>] [--type=<value>] [--length=<value>] [--comment=<value>]
key import [-h] [--name=<value>] [--comment=<value>]
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
usergroup create [-h] [--name=<value>] [--comment=<value>]
usergroup inspect [-h] USERGROUP...
usergroup ls [-h] [--latest] [--quiet]
usergroup rm [-h] USERGROUP...

# other
exit [-h]
help, h
info [-h]
version [-h]
```

---

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

---

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

---

## Scaling

`sshportal` is stateless but relies on a database to store configuration and logs.

By default, `sshportal` uses a local [sqlite](https://www.sqlite.org/) database which isn't scalable by design.

You can run multiple instances of `sshportal` sharing the same [MySQL](https://www.mysql.com) database, using `sshportal --db-conn=user:pass@host/dbname?parseTime=true --db-driver=mysql`.

![sshportal cluster with MySQL backend](https://raw.github.com/moul/sshportal/master/.assets/cluster-mysql.png)

See [examples/mysql](http://github.com/moul/sshportal/tree/master/examples/mysql).

---

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

![sshportal data model](https://raw.github.com/moul/sshportal/master/.assets/sql-schema.png)

---

## Testing

[Install golangci-lint](https://golangci-lint.run/usage/install/#local-installation) and run this in project root: 
```
golangci-lint run
```
---
Perform integration tests
```
make integration
```
---
Perform unit tests
```
make unittest
```
---

## Contributors

### Code Contributors

This project exists thanks to all the people who contribute. [[Contribute](CONTRIBUTING.md)].
<a href="https://github.com/moul/sshportal/graphs/contributors"><img src="https://opencollective.com/sshportal/contributors.svg?width=890&button=false" /></a>

### Financial Contributors

Become a financial contributor and help us sustain our community. [[Contribute](https://opencollective.com/sshportal/contribute)]

#### Individuals

<a href="https://opencollective.com/sshportal"><img src="https://opencollective.com/sshportal/individuals.svg?width=890"></a>

#### Organizations

Support this project with your organization. Your logo will show up here with a link to your website. [[Contribute](https://opencollective.com/sshportal/contribute)]

<a href="https://opencollective.com/sshportal/organization/0/website"><img src="https://opencollective.com/sshportal/organization/0/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/1/website"><img src="https://opencollective.com/sshportal/organization/1/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/2/website"><img src="https://opencollective.com/sshportal/organization/2/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/3/website"><img src="https://opencollective.com/sshportal/organization/3/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/4/website"><img src="https://opencollective.com/sshportal/organization/4/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/5/website"><img src="https://opencollective.com/sshportal/organization/5/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/6/website"><img src="https://opencollective.com/sshportal/organization/6/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/7/website"><img src="https://opencollective.com/sshportal/organization/7/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/8/website"><img src="https://opencollective.com/sshportal/organization/8/avatar.svg"></a>
<a href="https://opencollective.com/sshportal/organization/9/website"><img src="https://opencollective.com/sshportal/organization/9/avatar.svg"></a>

### Stargazers over time

[![Stargazers over time](https://starchart.cc/moul/sshportal.svg)](https://starchart.cc/moul/sshportal)
