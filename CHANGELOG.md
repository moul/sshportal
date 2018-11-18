# Changelog

## master (unreleased)

* Bump deps

## v1.9.0 (2018-11-18)

* Add `hostgroup update` and `usergroup update` commands ([#58](https://github.com/moul/sshportal/pull/58)) by [@adyxax](https://github.com/adyxax)
* Add socket timeout ([#80](https://github.com/moul/sshportal/pull/80)) by [@ahhx](https://github.com/ahhx)
* Add a flag to list only active sessions ([#76](https://github.com/moul/sshportal/pull/76)) by [@vdaviot](https://github.com/vdaviot)
* Unset hop on host ([#74](https://github.com/moul/sshportal/pull/74)) by [@vdaviot](https://github.com/vdaviot)
* Fix session status and duration display ([#75](https://github.com/moul/sshportal/pull/75)) by [@vdaviot](https://github.com/vdaviot)
* Fix log path and filename on Windows ([#78](https://github.com/moul/sshportal/pull/78)) by [@Raerten](https://github.com/Raerten)
* Admin user is not editable ([#69](https://github.com/moul/sshportal/pull/69)) by [@alenn-m](https://github.com/alenn-m)
* Switch to go modules (go1.11) ([#83](https://github.com/moul/sshportal/pull/83))
* Switch to moul.io/sshportal canonical URL ([#86](https://github.com/moul/sshportal/pull/86))
* Switch to golangci-lint ([#87](https://github.com/moul/sshportal/pull/87))

## v1.8.0 (2018-04-02)

* The default created user now has the same username as the user starting sshportal (was hardcoded "admin")
* Add Telnet support
* Add TTY audit feature ([#23](https://github.com/moul/sshportal/issues/23)) by [@sabban](https://github.com/sabban)
* Fix `--assign-*` commands when using MySQL driver ([#45](https://github.com/moul/sshportal/issues/45))
* Add *HOP* support, an efficient and integrated way of using a jump host transparently ([#47](https://github.com/moul/sshportal/issues/47)) by [@mathieui](https://github.com/mathieui)
* Fix panic on some `ls` commands ([#54](https://github.com/moul/sshportal/pull/54)) by [@jle64](https://github.com/jle64)
* Add tunnels (`direct-tcp`) support with logging ([#44](https://github.com/moul/sshportal/issues/44)) by [@sabban](https://github.com/sabban)
* Add `key import` command ([#52](https://github.com/moul/sshportal/issues/52)) by [@adyxax](https://github.com/adyxax)
* Add 'exec' logging ([#40](https://github.com/moul/sshportal/issues/40)) by [@sabban](https://github.com/sabban)

## v1.7.1 (2018-01-03)

* Return non-null exit-code on authentication error
* **hotfix**: repair invite system (broken in v1.7.0)

## v1.7.0 (2018-01-02)

Breaking changes:
* Use `sshportal server` instead of `sshportal` to start a new server (nothing to change if using the docker image)
* Remove `--config-user` and `--healthcheck-user` global options

Changes:
* Fix connection failure when sending too many environment variables (fix [#22](https://github.com/moul/sshportal/issues/22))
* Fix panic when entering empty command (fix [#13](https://github.com/moul/sshportal/issues/13))
* Add `config backup --ignore-events` option
* Add `sshportal healthcheck [--addr=] [--wait] [--quiet]` cli command
* Add [Docker Healthcheck](https://docs.docker.com/engine/reference/builder/#healthcheck) helper
* Support Putty (fix [#24](https://github.com/moul/sshportal/issues/24))

## v1.6.0 (2017-12-12)

* Add `--latest` and `--quiet` options to `ls` commands
* Add `healthcheck` user
* Add `key show KEY` command

## v1.5.0 (2017-12-02)

* Create Session objects on each connections (history)
* Connection history
* Audit log
* Add dynamic strict host key checking (learning on the first time, strict on the next ones)
* Add-back MySQL support (experimental)
* Fix some backup/restore bugs

## v1.4.0 (2017-11-24)

* Add 'key setup' command (easy SSH key installation)
* Add Updated and Created fields in 'ls' commands
* Add `--aes-key` option to encrypt sensitive data

## v1.3.0 (2017-11-23)

* More details in 'ls' commands
* Add 'host update' command (fix [#2](https://github.com/moul/sshportal/issues/2))
* Add 'user update' command (fix [#3](https://github.com/moul/sshportal/issues/3))
* Add 'acl update' command (fix [#4](https://github.com/moul/sshportal/issues/4))
* Allow connecting to the shell mode with the registered username or email (fix [#5](https://github.com/moul/sshportal/issues/5))
* Add 'listhosts' role (fix [#5](https://github.com/moul/sshportal/issues/5))

## v1.2.0 (2017-11-22)

* Support adding multiple `--group` links on `host create` and `user create`
* Use govalidator to perform more consistent input validation
* Use a database migration system

## v1.1.0 (2017-11-15)

* Improve versionning (static VERSION + dynamic GIT_* info)
* Configuration management (backup + restore)
* Implement Exit (fix [#6](https://github.com/moul/sshportal/pull/6))
* Disable mysql support (not fully working right now)
* Set random seed properly

## v1.0.0 (2017-11-14)

Initial version

* Host management
* User management
* User Group management
* Host Group management
* Host Key management
* User Key management
* ACL management
* Connect to host using key or password
* Admin commands can be run directly or in an interactive shell
