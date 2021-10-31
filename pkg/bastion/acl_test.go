package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"moul.io/sshportal/pkg/dbmodels"
)

func TestCheckACLs(t *testing.T) {
	Convey("Testing CheckACLs", t, func(c C) {
		// create tmp dir
		tempDir, err := ioutil.TempDir("", "sshportal")
		c.So(err, ShouldBeNil)
		defer func() {
			c.So(os.RemoveAll(tempDir), ShouldBeNil)
		}()

		// create sqlite db
		db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "sshportal.db")), &gorm.Config{})
		c.So(err, ShouldBeNil)
		c.So(DBInit(db), ShouldBeNil)

		// create dummy objects
		var hostGroup dbmodels.HostGroup
		err = dbmodels.HostGroupsByIdentifiers(db, []string{"default"}).First(&hostGroup).Error
		c.So(err, ShouldBeNil)
		db.Create(&dbmodels.Host{Groups: []*dbmodels.HostGroup{&hostGroup}})

		//. load db
		var (
			hosts []dbmodels.Host
			users []dbmodels.User
		)
		db.Preload("Groups").Preload("Groups.ACLs").Find(&hosts)
		db.Preload("Groups").Preload("Groups.ACLs").Find(&users)

		// test
		action := checkACLs(users[0], hosts[0], "")
		c.So(action, ShouldEqual, dbmodels.ACLActionAllow)
	})
}
