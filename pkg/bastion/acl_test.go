package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	. "github.com/smartystreets/goconvey/convey"
	"moul.io/sshportal/pkg/dbmodels"
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
		So(DBInit(db), ShouldBeNil)

		// create dummy objects
		var hostGroup dbmodels.HostGroup
		err = dbmodels.HostGroupsByIdentifiers(db, []string{"default"}).First(&hostGroup).Error
		So(err, ShouldBeNil)
		db.Create(&dbmodels.Host{Groups: []*dbmodels.HostGroup{&hostGroup}})

		//. load db
		var (
			hosts []dbmodels.Host
			users []dbmodels.User
		)
		db.Preload("Groups").Preload("Groups.ACLs").Find(&hosts)
		db.Preload("Groups").Preload("Groups.ACLs").Find(&users)

		// test
		action, err := checkACLs(users[0], hosts[0])
		So(err, ShouldBeNil)
		So(action, ShouldEqual, dbmodels.ACLActionAllow)
	})
}
