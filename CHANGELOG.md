# Changelog

## master (unreleased)

* Support adding multiple `--group` links on `host create` and `user create`
* Store and compare version in database
* Use govalidator to perform more consistent input validation

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
