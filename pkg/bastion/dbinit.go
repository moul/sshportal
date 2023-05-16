package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"os/user"
	"strings"
	"time"

	gormigrate "github.com/go-gormigrate/gormigrate/v2"
	gossh "golang.org/x/crypto/ssh"
	"gorm.io/gorm"
	"moul.io/sshportal/pkg/crypto"
	"moul.io/sshportal/pkg/dbmodels"
)

func DBInit(db *gorm.DB) error {
	log.SetOutput(ioutil.Discard)
	log.SetOutput(os.Stderr)

	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "1",
			Migrate: func(tx *gorm.DB) error {
				type Setting struct {
					gorm.Model
					Name  string `gorm:"index:uix_settings_name,unique"`
					Value string
				}
				return tx.AutoMigrate(&Setting{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("settings")
			},
		}, {
			ID: "2",
			Migrate: func(tx *gorm.DB) error {
				type SSHKey struct {
					gorm.Model
					Name        string
					Type        string
					Length      uint
					Fingerprint string
					PrivKey     string           `sql:"size:5000"`
					PubKey      string           `sql:"size:1000"`
					Hosts       []*dbmodels.Host `gorm:"ForeignKey:SSHKeyID"`
					Comment     string
				}
				return tx.AutoMigrate(&SSHKey{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("ssh_keys")
			},
		}, {
			ID: "3",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					gorm.Model
					Name        string `gorm:"size:32"`
					Addr        string
					User        string
					Password    string
					SSHKey      *dbmodels.SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID    uint                  `gorm:"index"`
					Groups      []*dbmodels.HostGroup `gorm:"many2many:host_host_groups;"`
					Fingerprint string
					Comment     string
				}
				return tx.AutoMigrate(&Host{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("hosts")
			},
		}, {
			ID: "4",
			Migrate: func(tx *gorm.DB) error {
				type UserKey struct {
					gorm.Model
					Key     []byte         `sql:"size:1000"`
					UserID  uint           ``
					User    *dbmodels.User `gorm:"ForeignKey:UserID"`
					Comment string
				}
				return tx.AutoMigrate(&UserKey{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("user_keys")
			},
		}, {
			ID: "5",
			Migrate: func(tx *gorm.DB) error {
				type User struct {
					gorm.Model
					IsAdmin     bool
					Email       string
					Name        string
					Keys        []*dbmodels.UserKey   `gorm:"ForeignKey:UserID"`
					Groups      []*dbmodels.UserGroup `gorm:"many2many:user_user_groups;"`
					Comment     string
					InviteToken string
				}
				return tx.AutoMigrate(&User{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("users")
			},
		}, {
			ID: "6",
			Migrate: func(tx *gorm.DB) error {
				type UserGroup struct {
					gorm.Model
					Name    string
					Users   []*dbmodels.User `gorm:"many2many:user_user_groups;"`
					ACLs    []*dbmodels.ACL  `gorm:"many2many:user_group_acls;"`
					Comment string
				}
				return tx.AutoMigrate(&UserGroup{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("user_groups")
			},
		}, {
			ID: "7",
			Migrate: func(tx *gorm.DB) error {
				type HostGroup struct {
					gorm.Model
					Name    string
					Hosts   []*dbmodels.Host `gorm:"many2many:host_host_groups;"`
					ACLs    []*dbmodels.ACL  `gorm:"many2many:host_group_acls;"`
					Comment string
				}
				return tx.AutoMigrate(&HostGroup{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("host_groups")
			},
		}, {
			ID: "8",
			Migrate: func(tx *gorm.DB) error {
				type ACL struct {
					gorm.Model
					HostGroups  []*dbmodels.HostGroup `gorm:"many2many:host_group_acls;"`
					UserGroups  []*dbmodels.UserGroup `gorm:"many2many:user_group_acls;"`
					HostPattern string
					Action      string
					Weight      uint
					Comment     string
				}

				return tx.AutoMigrate(&ACL{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("acls")
			},
		}, {
			ID: "9",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&dbmodels.Setting{}, "uix_settings_name"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&dbmodels.Setting{}, "uix_settings_name")
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropIndex(&dbmodels.Setting{}, "uix_settings_name")
			},
		}, {
			ID: "10",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&dbmodels.SSHKey{}, "uix_keys_name"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&dbmodels.SSHKey{}, "uix_keys_name")
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropIndex(&dbmodels.SSHKey{}, "uix_keys_name")
			},
		}, {
			ID: "11",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&dbmodels.Host{}, "uix_hosts_name"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&dbmodels.Host{}, "uix_hosts_name")
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropIndex(&dbmodels.Host{}, "uix_hosts_name")
			},
		}, {
			ID: "12",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&dbmodels.User{}, "uix_users_name"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&dbmodels.User{}, "uix_users_name")
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropIndex(&dbmodels.User{}, "uix_users_name")
			},
		}, {
			ID: "13",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&dbmodels.UserGroup{}, "uix_usergroups_name"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&dbmodels.UserGroup{}, "uix_usergroups_name")
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropIndex(&dbmodels.UserGroup{}, "uix_usergroups_name")
			},
		}, {
			ID: "14",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&dbmodels.HostGroup{}, "uix_hostgroups_name"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&dbmodels.HostGroup{}, "uix_hostgroups_name")
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropIndex(&dbmodels.HostGroup{}, "uix_hostgroups_name")
			},
		}, {
			ID: "15",
			Migrate: func(tx *gorm.DB) error {
				type UserRole struct {
					gorm.Model
					Name  string           `valid:"required,length(1|32),unix_user"`
					Users []*dbmodels.User `gorm:"many2many:user_user_roles"`
				}
				return tx.AutoMigrate(&UserRole{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("user_roles")
			},
		}, {
			ID: "16",
			Migrate: func(tx *gorm.DB) error {
				type User struct {
					gorm.Model
					IsAdmin     bool
					Roles       []*dbmodels.UserRole  `gorm:"many2many:user_user_roles"`
					Email       string                `valid:"required,email"`
					Name        string                `valid:"required,length(1|32),unix_user"`
					Keys        []*dbmodels.UserKey   `gorm:"ForeignKey:UserID"`
					Groups      []*dbmodels.UserGroup `gorm:"many2many:user_user_groups;"`
					Comment     string                `valid:"optional"`
					InviteToken string                `valid:"optional,length(10|60)"`
				}
				return tx.AutoMigrate(&User{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "17",
			Migrate: func(tx *gorm.DB) error {
				return tx.Create(&dbmodels.UserRole{Name: "admin"}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Where("name = ?", "admin").Unscoped().Delete(&dbmodels.UserRole{}).Error
			},
		}, {
			ID: "18",
			Migrate: func(tx *gorm.DB) error {
				var adminRole dbmodels.UserRole
				if err := db.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
					return err
				}

				var users []*dbmodels.User
				if err := db.Preload("Roles").Where("is_admin = ?", true).Find(&users).Error; err != nil {
					return err
				}

				for _, user := range users {
					user.Roles = append(user.Roles, &adminRole)
					if err := tx.Save(user).Error; err != nil {
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
					Roles       []*dbmodels.UserRole  `gorm:"many2many:user_user_roles"`
					Email       string                `valid:"required,email"`
					Name        string                `valid:"required,length(1|32),unix_user"`
					Keys        []*dbmodels.UserKey   `gorm:"ForeignKey:UserID"`
					Groups      []*dbmodels.UserGroup `gorm:"many2many:user_user_groups;"`
					Comment     string                `valid:"optional"`
					InviteToken string                `valid:"optional,length(10|60)"`
				}
				return tx.AutoMigrate(&User{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "20",
			Migrate: func(tx *gorm.DB) error {
				return tx.Create(&dbmodels.UserRole{Name: "listhosts"}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Where("name = ?", "listhosts").Unscoped().Delete(&dbmodels.UserRole{}).Error
			},
		}, {
			ID: "21",
			Migrate: func(tx *gorm.DB) error {
				type Session struct {
					gorm.Model
					StoppedAt time.Time      `valid:"optional"`
					Status    string         `valid:"required"`
					User      *dbmodels.User `gorm:"ForeignKey:UserID"`
					Host      *dbmodels.Host `gorm:"ForeignKey:HostID"`
					UserID    uint           `valid:"optional"`
					HostID    uint           `valid:"optional"`
					ErrMsg    string         `valid:"optional"`
					Comment   string         `valid:"optional"`
				}
				return tx.AutoMigrate(&Session{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("sessions")
			},
		}, {
			ID: "22",
			Migrate: func(tx *gorm.DB) error {
				type Event struct {
					gorm.Model
					Author   *dbmodels.User `gorm:"ForeignKey:AuthorID"`
					AuthorID uint           `valid:"optional"`
					Domain   string         `valid:"required"`
					Action   string         `valid:"required"`
					Entity   string         `valid:"optional"`
					Args     []byte         `sql:"size:10000" valid:"optional,length(1|10000)"`
				}
				return tx.AutoMigrate(&Event{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("events")
			},
		}, {
			ID: "23",
			Migrate: func(tx *gorm.DB) error {
				type UserKey struct {
					gorm.Model
					Key           []byte         `sql:"size:1000" valid:"required,length(1|1000)"`
					AuthorizedKey string         `sql:"size:1000" valid:"required,length(1|1000)"`
					UserID        uint           ``
					User          *dbmodels.User `gorm:"ForeignKey:UserID"`
					Comment       string         `valid:"optional"`
				}
				return tx.AutoMigrate(&UserKey{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "24",
			Migrate: func(tx *gorm.DB) error {
				var userKeys []*dbmodels.UserKey
				if err := db.Find(&userKeys).Error; err != nil {
					return err
				}

				for _, userKey := range userKeys {
					key, err := gossh.ParsePublicKey(userKey.Key)
					if err != nil {
						return err
					}
					userKey.AuthorizedKey = string(gossh.MarshalAuthorizedKey(key))
					if err := db.Model(userKey).Updates(userKey).Error; err != nil {
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
					gorm.Model
					Name        string                `gorm:"size:32" valid:"required,length(1|32),unix_user"`
					Addr        string                `valid:"required"`
					User        string                `valid:"optional"`
					Password    string                `valid:"optional"`
					SSHKey      *dbmodels.SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID    uint                  `gorm:"index"`
					HostKey     []byte                `sql:"size:1000" valid:"optional"`
					Groups      []*dbmodels.HostGroup `gorm:"many2many:host_host_groups;"`
					Fingerprint string                `valid:"optional"`
					Comment     string                `valid:"optional"`
				}
				return tx.AutoMigrate(&Host{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "26",
			Migrate: func(tx *gorm.DB) error {
				type Session struct {
					gorm.Model
					StoppedAt *time.Time     `sql:"index" valid:"optional"`
					Status    string         `valid:"required"`
					User      *dbmodels.User `gorm:"ForeignKey:UserID"`
					Host      *dbmodels.Host `gorm:"ForeignKey:HostID"`
					UserID    uint           `valid:"optional"`
					HostID    uint           `valid:"optional"`
					ErrMsg    string         `valid:"optional"`
					Comment   string         `valid:"optional"`
				}
				return tx.AutoMigrate(&Session{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "27",
			Migrate: func(tx *gorm.DB) error {
				var sessions []*dbmodels.Session
				if err := db.Find(&sessions).Error; err != nil {
					return err
				}

				for _, session := range sessions {
					if session.StoppedAt != nil && session.StoppedAt.IsZero() {
						if err := db.Model(session).Updates(map[string]interface{}{"stopped_at": nil}).Error; err != nil {
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
					gorm.Model
					Name     string `gorm:"size:32"`
					Addr     string
					User     string
					Password string
					URL      string
					SSHKey   *dbmodels.SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID uint                  `gorm:"index"`
					HostKey  []byte                `sql:"size:1000"`
					Groups   []*dbmodels.HostGroup `gorm:"many2many:host_host_groups;"`
					Comment  string
				}
				return tx.AutoMigrate(&Host{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "29",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					gorm.Model
					Name     string `gorm:"size:32"`
					Addr     string
					User     string
					Password string
					URL      string
					SSHKey   *dbmodels.SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID uint                  `gorm:"index"`
					HostKey  []byte                `sql:"size:1000"`
					Groups   []*dbmodels.HostGroup `gorm:"many2many:host_host_groups;"`
					Comment  string
					Hop      *dbmodels.Host
					HopID    uint
				}
				return tx.AutoMigrate(&Host{})
			},
			Rollback: func(tx *gorm.DB) error {
				return fmt.Errorf("not implemented")
			},
		}, {
			ID: "30",
			Migrate: func(tx *gorm.DB) error {
				type Host struct {
					gorm.Model
					Name     string `gorm:"size:32"`
					Addr     string
					User     string
					Password string
					URL      string
					SSHKey   *dbmodels.SSHKey      `gorm:"ForeignKey:SSHKeyID"`
					SSHKeyID uint                  `gorm:"index"`
					HostKey  []byte                `sql:"size:10000"`
					Groups   []*dbmodels.HostGroup `gorm:"many2many:host_host_groups;"`
					Comment  string
					Hop      *dbmodels.Host
					Logging  string
					HopID    uint
				}
				return tx.AutoMigrate(&Host{})
			},
			Rollback: func(tx *gorm.DB) error { return fmt.Errorf("not implemented") },
		}, {
			ID: "31",
			Migrate: func(tx *gorm.DB) error {
				return tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Model(&dbmodels.Host{}).Updates(&dbmodels.Host{Logging: "everything"}).Error
			},
			Rollback: func(tx *gorm.DB) error { return fmt.Errorf("not implemented") },
		}, {
			ID: "32",
			Migrate: func(tx *gorm.DB) error {
				type ACL struct {
					gorm.Model
					HostGroups  []*dbmodels.HostGroup `gorm:"many2many:host_group_acls;"`
					UserGroups  []*dbmodels.UserGroup `gorm:"many2many:user_group_acls;"`
					HostPattern string                `valid:"optional"`
					Action      string                `valid:"required"`
					Weight      uint                  ``
					Comment     string                `valid:"optional"`
					Inception   *time.Time
					Expiration  *time.Time
				}
				return tx.AutoMigrate(&ACL{})
			},
			Rollback: func(tx *gorm.DB) error { return fmt.Errorf("not implemented") },
		},
	})
	if err := m.Migrate(); err != nil {
		return err
	}
	dbmodels.NewEvent("system", "migrated").Log(db)

	// create default ssh key
	var count int64
	if err := db.Table("ssh_keys").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		key, err := crypto.NewSSHKey("ed25519", 1)
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
		hostGroup := dbmodels.HostGroup{
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
		userGroup := dbmodels.UserGroup{
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
		var defaultUserGroup dbmodels.UserGroup
		db.Where("name = ?", "default").First(&defaultUserGroup)
		var defaultHostGroup dbmodels.HostGroup
		db.Where("name = ?", "default").First(&defaultHostGroup)
		acl := dbmodels.ACL{
			UserGroups: []*dbmodels.UserGroup{&defaultUserGroup},
			HostGroups: []*dbmodels.HostGroup{&defaultHostGroup},
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
	var defaultUserGroup dbmodels.UserGroup
	db.Where("name = ?", "default").First(&defaultUserGroup)
	if err := db.Table("users").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		// if no admin, create an account for the first connection
		inviteToken, err := randStringBytes(16)
		if err != nil {
			return err
		}
		if os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN") != "" {
			inviteToken = os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN")
		}
		var adminRole dbmodels.UserRole
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
		user := dbmodels.User{
			Name:        username,
			Email:       fmt.Sprintf("%s@localhost", username),
			Comment:     "created by sshportal",
			Roles:       []*dbmodels.UserRole{&adminRole},
			InviteToken: inviteToken,
			Groups:      []*dbmodels.UserGroup{&defaultUserGroup},
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
		key, err := crypto.NewSSHKey("ed25519", 1)
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
	return db.Table("sessions").Where("status = ?", "active").Updates(&dbmodels.Session{
		Status: string(dbmodels.SessionStatusClosed),
		ErrMsg: "sshportal was halted while the connection was still active",
	}).Error
}

func randStringBytes(n int) (string, error) {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	for i := range b {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterBytes))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random string: %s", err)
		}
		b[i] = letterBytes[r.Int64()]
	}
	return string(b), nil
}
