package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/jinzhu/gorm"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckACLs(t *testing.T) {
	Convey("Testing CheckACLs", t, func() {
		// create tmp dir
		tempDir, err := ioutil.TempDir("", "sshportal")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		// create sqlite db
		db, err := gorm.Open("sqlite3", filepath.Join(tempDir, "sshportal.db"))
		db.LogMode(false)
		So(dbInit(db), ShouldBeNil)

		// create dummy objects
		hostGroup, err := FindHostGroupByIdOrName(db, "default")
		So(err, ShouldBeNil)
		db.Create(&Host{Groups: []HostGroup{*hostGroup}})

		//. load db
		var (
			hosts []Host
			users []User
		)
		db.Preload("Groups").Preload("Groups.ACLs").Find(&hosts)
		db.Preload("Groups").Preload("Groups.ACLs").Find(&users)

		// test
		action, err := CheckACLs(users[0], hosts[0])
		So(err, ShouldBeNil)
		So(action, ShouldEqual, "allow")
	})
}
