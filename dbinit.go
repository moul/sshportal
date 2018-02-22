package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/go-gormigrate/gormigrate"
	"github.com/jinzhu/gorm"
	gossh "golang.org/x/crypto/ssh"
)

func dbInit(db *gorm.DB) error {
	log.SetOutput(ioutil.Discard)
	db.Callback().Delete().Replace("gorm:delete", hardDeleteCallback)
	log.SetOutput(os.Stderr)

	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "1",
			Migrate: func(tx *gorm.DB) error {
				type Setting struct {
					gorm.Model
					Name  string
					Value string
				}
				return tx.AutoMigrate(&Setting{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("settings").Error
			},
		}, {
			ID: "2",
			Migrate: func(tx *gorm.DB) error {
				type SSHKey struct {
					// FIXME: use uuid for ID
					gorm.Model
					Name        string
					Type        string
					Length      uint
					Fingerprint string
					PrivKey     string  `sql:"size:10000"`
					PubKey      string  `sql:"size:10000"`
					Hosts       []*Host `gorm:"ForeignKey:SSHKeyID"`
					Comment     string
				}
				return tx.AutoMigrate(&SSHKey{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("ssh_keys").Error
			},
		}, {
			ID: "3",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					// FIXME: use uuid for ID
					gorm.Model
					Name        string `gorm:"size:32"`
					Addr        string
					User        string
					Password    string
					SSHKey      *SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID    uint         `gorm:"index"`
					Groups      []*HostGroup `gorm:"many2many:host_host_groups;"`
					Fingerprint string
					Comment     string
				}
				return tx.AutoMigrate(&Host{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("hosts").Error
			},
		}, {
			ID: "4",
			Migrate: func(tx *gorm.DB) error {
				type UserKey struct {
					gorm.Model
					Key     []byte `sql:"size:10000"`
					UserID  uint   ``
					User    *User  `gorm:"ForeignKey:UserID"`
					Comment string
				}
				return tx.AutoMigrate(&UserKey{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("user_keys").Error
			},
		}, {
			ID: "5",
			Migrate: func(tx *gorm.DB) error {
				type User struct {
					// FIXME: use uuid for ID
					gorm.Model
					IsAdmin     bool
					Email       string
					Name        string
					Keys        []*UserKey   `gorm:"ForeignKey:UserID"`
					Groups      []*UserGroup `gorm:"many2many:user_user_groups;"`
					Comment     string
					InviteToken string
				}
				return tx.AutoMigrate(&User{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("users").Error
			},
		}, {
			ID: "6",
			Migrate: func(tx *gorm.DB) error {
				type UserGroup struct {
					gorm.Model
					Name    string
					Users   []*User `gorm:"many2many:user_user_groups;"`
					ACLs    []*ACL  `gorm:"many2many:user_group_acls;"`
					Comment string
				}
				return tx.AutoMigrate(&UserGroup{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("user_groups").Error
			},
		}, {
			ID: "7",
			Migrate: func(tx *gorm.DB) error {
				type HostGroup struct {
					gorm.Model
					Name    string
					Hosts   []*Host `gorm:"many2many:host_host_groups;"`
					ACLs    []*ACL  `gorm:"many2many:host_group_acls;"`
					Comment string
				}
				return tx.AutoMigrate(&HostGroup{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("host_groups").Error
			},
		}, {
			ID: "8",
			Migrate: func(tx *gorm.DB) error {
				type ACL struct {
					gorm.Model
					HostGroups  []*HostGroup `gorm:"many2many:host_group_acls;"`
					UserGroups  []*UserGroup `gorm:"many2many:user_group_acls;"`
					HostPattern string
					Action      string
					Weight      uint
					Comment     string
				}

				return tx.AutoMigrate(&ACL{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("acls").Error
			},
		}, {
			ID: "9",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&Setting{}).RemoveIndex("uix_settings_name")
				return db.Model(&Setting{}).AddUniqueIndex("uix_settings_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&Setting{}).RemoveIndex("uix_settings_name").Error
			},
		}, {
			ID: "10",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&SSHKey{}).RemoveIndex("uix_keys_name")
				return db.Model(&SSHKey{}).AddUniqueIndex("uix_keys_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&SSHKey{}).RemoveIndex("uix_keys_name").Error
			},
		}, {
			ID: "11",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&Host{}).RemoveIndex("uix_hosts_name")
				return db.Model(&Host{}).AddUniqueIndex("uix_hosts_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&Host{}).RemoveIndex("uix_hosts_name").Error
			},
		}, {
			ID: "12",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&User{}).RemoveIndex("uix_users_name")
				return db.Model(&User{}).AddUniqueIndex("uix_users_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&User{}).RemoveIndex("uix_users_name").Error
			},
		}, {
			ID: "13",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&UserGroup{}).RemoveIndex("uix_usergroups_name")
				return db.Model(&UserGroup{}).AddUniqueIndex("uix_usergroups_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&UserGroup{}).RemoveIndex("uix_usergroups_name").Error
			},
		}, {
			ID: "14",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&HostGroup{}).RemoveIndex("uix_hostgroups_name")
				return db.Model(&HostGroup{}).AddUniqueIndex("uix_hostgroups_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&HostGroup{}).RemoveIndex("uix_hostgroups_name").Error
			},
		}, {
			ID: "15",
			Migrate: func(tx *gorm.DB) error {
				type UserRole struct {
					gorm.Model
					Name  string  `valid:"required,length(1|32),unix_user"`
					Users []*User `gorm:"many2many:user_user_roles"`
				}
				return tx.AutoMigrate(&UserRole{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("user_roles").Error
			},
		}, {
			ID: "16",
			Migrate: func(tx *gorm.DB) error {
				type User struct {
					gorm.Model
					IsAdmin     bool
					Roles       []*UserRole  `gorm:"many2many:user_user_roles"`
					Email       string       `valid:"required,email"`
					Name        string       `valid:"required,length(1|32),unix_user"`
					Keys        []*UserKey   `gorm:"ForeignKey:UserID"`
					Groups      []*UserGroup `gorm:"many2many:user_user_groups;"`
					Comment     string       `valid:"optional"`
					InviteToken string       `valid:"optional,length(10|60)"`
				}
				return tx.AutoMigrate(&User{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "17",
			Migrate: func(tx *gorm.DB) error {
				return tx.Create(&UserRole{Name: "admin"}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Where("name = ?", "admin").Delete(&UserRole{}).Error
			},
		}, {
			ID: "18",
			Migrate: func(tx *gorm.DB) error {
				var adminRole UserRole
				if err := db.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
					return err
				}

				var users []User
				if err := db.Preload("Roles").Where("is_admin = ?", true).Find(&users).Error; err != nil {
					return err
				}

				for _, user := range users {
					user.Roles = append(user.Roles, &adminRole)
					if err := tx.Save(&user).Error; err != nil {
						return err
					}
				}
				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "19",
			Migrate: func(tx *gorm.DB) error {
				type User struct {
					gorm.Model
					Roles       []*UserRole  `gorm:"many2many:user_user_roles"`
					Email       string       `valid:"required,email"`
					Name        string       `valid:"required,length(1|32),unix_user"`
					Keys        []*UserKey   `gorm:"ForeignKey:UserID"`
					Groups      []*UserGroup `gorm:"many2many:user_user_groups;"`
					Comment     string       `valid:"optional"`
					InviteToken string       `valid:"optional,length(10|60)"`
				}
				return tx.AutoMigrate(&User{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "20",
			Migrate: func(tx *gorm.DB) error {
				return tx.Create(&UserRole{Name: "listhosts"}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Where("name = ?", "listhosts").Delete(&UserRole{}).Error
			},
		}, {
			ID: "21",
			Migrate: func(tx *gorm.DB) error {
				type Session struct {
					gorm.Model
					StoppedAt time.Time `valid:"optional"`
					Status    string    `valid:"required"`
					User      *User     `gorm:"ForeignKey:UserID"`
					Host      *Host     `gorm:"ForeignKey:HostID"`
					UserID    uint      `valid:"optional"`
					HostID    uint      `valid:"optional"`
					ErrMsg    string    `valid:"optional"`
					Comment   string    `valid:"optional"`
				}
				return tx.AutoMigrate(&Session{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("sessions").Error
			},
		}, {
			ID: "22",
			Migrate: func(tx *gorm.DB) error {
				type Event struct {
					gorm.Model
					Author   *User  `gorm:"ForeignKey:AuthorID"`
					AuthorID uint   `valid:"optional"`
					Domain   string `valid:"required"`
					Action   string `valid:"required"`
					Entity   string `valid:"optional"`
					Args     []byte `sql:"size:10000" valid:"optional,length(1|10000)"`
				}
				return tx.AutoMigrate(&Event{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("events").Error
			},
		}, {
			ID: "23",
			Migrate: func(tx *gorm.DB) error {
				type UserKey struct {
					gorm.Model
					Key           []byte `sql:"size:10000" valid:"required,length(1|10000)"`
					AuthorizedKey string `sql:"size:10000" valid:"required,length(1|10000)"`
					UserID        uint   ``
					User          *User  `gorm:"ForeignKey:UserID"`
					Comment       string `valid:"optional"`
				}
				return tx.AutoMigrate(&UserKey{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "24",
			Migrate: func(tx *gorm.DB) error {
				var userKeys []UserKey
				if err := db.Find(&userKeys).Error; err != nil {
					return err
				}

				for _, userKey := range userKeys {
					key, err := gossh.ParsePublicKey(userKey.Key)
					if err != nil {
						return err
					}
					userKey.AuthorizedKey = string(gossh.MarshalAuthorizedKey(key))
					if err := db.Model(&userKey).Updates(&userKey).Error; err != nil {
						return err
					}
				}
				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "25",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					// FIXME: use uuid for ID
					gorm.Model
					Name        string       `gorm:"size:32" valid:"required,length(1|32),unix_user"`
					Addr        string       `valid:"required"`
					User        string       `valid:"optional"`
					Password    string       `valid:"optional"`
					SSHKey      *SSHKey      `gorm:"ForeignKey:SSHKeyID"` // SSHKey used to connect by the client
					SSHKeyID    uint         `gorm:"index"`
					HostKey     []byte       `sql:"size:10000" valid:"optional"`
					Groups      []*HostGroup `gorm:"many2many:host_host_groups;"`
					Fingerprint string       `valid:"optional"` // FIXME: replace with hostKey ?
					Comment     string       `valid:"optional"`
				}
				return tx.AutoMigrate(&Host{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "26",
			Migrate: func(tx *gorm.DB) error {
				type Session struct {
					gorm.Model
					StoppedAt *time.Time `sql:"index" valid:"optional"`
					Status    string     `valid:"required"`
					User      *User      `gorm:"ForeignKey:UserID"`
					Host      *Host      `gorm:"ForeignKey:HostID"`
					UserID    uint       `valid:"optional"`
					HostID    uint       `valid:"optional"`
					ErrMsg    string     `valid:"optional"`
					Comment   string     `valid:"optional"`
				}
				return tx.AutoMigrate(&Session{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "27",
			Migrate: func(tx *gorm.DB) error {
				var sessions []Session
				if err := db.Find(&sessions).Error; err != nil {
					return err
				}

				for _, session := range sessions {
					if session.StoppedAt != nil && session.StoppedAt.IsZero() {
						if err := db.Model(&session).Updates(map[string]interface{}{"stopped_at": nil}).Error; err != nil {
							return err
						}
					}
				}
				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "28",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					// FIXME: use uuid for ID
					gorm.Model
					Name     string `gorm:"size:32"`
					Addr     string
					User     string
					Password string
					URL      string
					SSHKey   *SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID uint         `gorm:"index"`
					HostKey  []byte       `sql:"size:10000"`
					Groups   []*HostGroup `gorm:"many2many:host_host_groups;"`
					Comment  string
				}
				return tx.AutoMigrate(&Host{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "29",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					// FIXME: use uuid for ID
					gorm.Model
					Name     string `gorm:"size:32"`
					Addr     string
					User     string
					Password string
					URL      string
					SSHKey   *SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID uint         `gorm:"index"`
					HostKey  []byte       `sql:"size:10000"`
					Groups   []*HostGroup `gorm:"many2many:host_host_groups;"`
					Comment  string
					Hop      *Host
					HopID    uint
				}
				return tx.AutoMigrate(&Host{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		},
	})
	if err := m.Migrate(); err != nil {
		return err
	}
	NewEvent("system", "migrated").Log(db)

	// create default ssh key
	var count uint
	if err := db.Table("ssh_keys").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		key, err := NewSSHKey("rsa", 2048)
		if err != nil {
			return err
		}
		key.Name = "default"
		key.Comment = "created by sshportal"
		if err := db.Create(&key).Error; err != nil {
			return err
		}
	}

	// create default host group
	if err := db.Table("host_groups").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		hostGroup := HostGroup{
			Name:    "default",
			Comment: "created by sshportal",
		}
		if err := db.Create(&hostGroup).Error; err != nil {
			return err
		}
	}

	// create default user group
	if err := db.Table("user_groups").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		userGroup := UserGroup{
			Name:    "default",
			Comment: "created by sshportal",
		}
		if err := db.Create(&userGroup).Error; err != nil {
			return err
		}
	}

	// create default acl
	if err := db.Table("acls").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		var defaultUserGroup UserGroup
		db.Where("name = ?", "default").First(&defaultUserGroup)
		var defaultHostGroup HostGroup
		db.Where("name = ?", "default").First(&defaultHostGroup)
		acl := ACL{
			UserGroups: []*UserGroup{&defaultUserGroup},
			HostGroups: []*HostGroup{&defaultHostGroup},
			Action:     "allow",
			//HostPattern: "",
			//Weight:      0,
			Comment: "created by sshportal",
		}
		if err := db.Create(&acl).Error; err != nil {
			return err
		}
	}

	// create admin user
	var defaultUserGroup UserGroup
	db.Where("name = ?", "default").First(&defaultUserGroup)
	if err := db.Table("users").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		// if no admin, create an account for the first connection
		inviteToken := randStringBytes(16)
		if os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN") != "" {
			inviteToken = os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN")
		}
		var adminRole UserRole
		if err := db.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
			return err
		}
		var username string
		if currentUser, err := user.Current(); err == nil {
			username = currentUser.Username
		}
		if username == "" {
			username = os.Getenv("USER")
		}
		username = strings.ToLower(username)
		if username == "" {
			username = "admin" // fallback username
		}
		user := User{
			Name:        username,
			Email:       fmt.Sprintf("%s@localhost", username),
			Comment:     "created by sshportal",
			Roles:       []*UserRole{&adminRole},
			InviteToken: inviteToken,
			Groups:      []*UserGroup{&defaultUserGroup},
		}
		if err := db.Create(&user).Error; err != nil {
			return err
		}
		log.Printf("info 'admin' user created, use the user 'invite:%s' to associate a public key with this account", user.InviteToken)
	}

	// create host ssh key
	if err := db.Table("ssh_keys").Where("name = ?", "host").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		key, err := NewSSHKey("rsa", 2048)
		if err != nil {
			return err
		}
		key.Name = "host"
		key.Comment = "created by sshportal"
		if err := db.Create(&key).Error; err != nil {
			return err
		}
	}

	// close unclosed connections
	return db.Table("sessions").Where("status = ?", "active").Updates(&Session{
		Status: SessionStatusClosed,
		ErrMsg: "sshportal was halted while the connection was still active",
	}).Error
}

func hardDeleteCallback(scope *gorm.Scope) {
	if !scope.HasError() {
		var extraOption string
		if str, ok := scope.Get("gorm:delete_option"); ok {
			extraOption = fmt.Sprint(str)
		}

		/* #nosec */
		scope.Raw(fmt.Sprintf(
			"DELETE FROM %v%v%v",
			scope.QuotedTableName(),
			addExtraSpaceIfExist(scope.CombinedConditionSql()),
			addExtraSpaceIfExist(extraOption),
		)).Exec()
	}
}

func addExtraSpaceIfExist(str string) string {
	if str != "" {
		return " " + str
	}
	return ""
}
