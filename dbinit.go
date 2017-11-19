package main

import (
	"log"
	"os"

	"github.com/go-gormigrate/gormigrate"
	"github.com/jinzhu/gorm"
)

func dbInit(db *gorm.DB) error {
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
		},
		{
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
				return db.Model(&Setting{}).Where(`"deleted_at" IS NULL`).AddUniqueIndex("uix_settings_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&Setting{}).RemoveIndex("uix_settings_name").Error
			},
		}, {
			ID: "10",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&SSHKey{}).RemoveIndex("uix_keys_name")
				return db.Model(&SSHKey{}).Where(`"deleted_at" IS NULL`).AddUniqueIndex("uix_keys_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&SSHKey{}).RemoveIndex("uix_keys_name").Error
			},
		}, {
			ID: "11",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&Host{}).RemoveIndex("uix_hosts_name")
				return db.Model(&Host{}).Where(`"deleted_at" IS NULL`).AddUniqueIndex("uix_hosts_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&Host{}).RemoveIndex("uix_hosts_name").Error
			},
		}, {
			ID: "12",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&User{}).RemoveIndex("uix_users_name")
				return db.Model(&User{}).Where(`"deleted_at" IS NULL`).AddUniqueIndex("uix_users_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&User{}).RemoveIndex("uix_users_name").Error
			},
		}, {
			ID: "13",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&UserGroup{}).RemoveIndex("uix_usergroups_name")
				return db.Model(&UserGroup{}).Where(`"deleted_at" IS NULL`).AddUniqueIndex("uix_usergroups_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&UserGroup{}).RemoveIndex("uix_usergroups_name").Error
			},
		}, {
			ID: "14",
			Migrate: func(tx *gorm.DB) error {
				db.Model(&HostGroup{}).RemoveIndex("uix_hostgroups_name")
				return db.Model(&HostGroup{}).Where(`"deleted_at" IS NULL`).AddUniqueIndex("uix_hostgroups_name", "name").Error
			},
			Rollback: func(tx *gorm.DB) error {
				return db.Model(&HostGroup{}).RemoveIndex("uix_hostgroups_name").Error
			},
		},
	})
	if err := m.Migrate(); err != nil {
		return err
	}

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
	db.Table("users").Count(&count)
	if count == 0 {
		// if no admin, create an account for the first connection
		inviteToken := RandStringBytes(16)
		if os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN") != "" {
			inviteToken = os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN")
		}
		user := User{
			Name:        "Administrator",
			Email:       "admin@sshportal",
			Comment:     "created by sshportal",
			IsAdmin:     true,
			InviteToken: inviteToken,
			Groups:      []*UserGroup{&defaultUserGroup},
		}
		db.Create(&user)
		log.Printf("Admin user created, use the user 'invite:%s' to associate a public key with this account", user.InviteToken)
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
	return nil
}

func dbDemo(db *gorm.DB) error {
	hostGroup, err := FindHostGroupByIdOrName(db, "default")
	if err != nil {
		return err
	}

	key, err := FindKeyByIdOrName(db, "default")
	if err != nil {
		return err
	}

	var (
		host1 = Host{Name: "sdf", Addr: "sdf.org:22", User: "new", SSHKeyID: key.ID, Groups: []*HostGroup{hostGroup}}
		host2 = Host{Name: "whoami", Addr: "whoami.filippo.io:22", User: "test", SSHKeyID: key.ID, Groups: []*HostGroup{hostGroup}}
		host3 = Host{Name: "ssh-chat", Addr: "chat.shazow.net:22", User: "test", SSHKeyID: key.ID, Fingerprint: "MD5:e5:d5:d1:75:90:38:42:f6:c7:03:d7:d0:56:7d:6a:db", Groups: []*HostGroup{hostGroup}}
	)

	// FIXME: check if hosts exist to avoid `UNIQUE constraint` error
	db.FirstOrCreate(&host1)
	db.FirstOrCreate(&host2)
	db.FirstOrCreate(&host3)
	return nil
}
