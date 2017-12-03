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
		defer func() {
			So(os.RemoveAll(tempDir), ShouldBeNil)
		}()

		// create sqlite db
		db, err := gorm.Open("sqlite3", filepath.Join(tempDir, "sshportal.db"))
		So(err, ShouldBeNil)
		db.LogMode(false)
		So(dbInit(db), ShouldBeNil)

		// create dummy objects
		var hostGroup HostGroup
		err = HostGroupsByIdentifiers(db, []string{"default"}).First(&hostGroup).Error
		So(err, ShouldBeNil)
		db.Create(&Host{Groups: []*HostGroup{&hostGroup}})

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
		So(action, ShouldEqual, ACLActionAllow)
	})
}
