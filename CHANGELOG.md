# Changelog

## master (unreleased)

* Add 'key setup' command (easy SSH key installation)
* Add Updated and Created fields in 'ls' commands

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
