package main

import "github.com/jinzhu/gorm"

type Key struct {
	gorm.Model
}

type Host struct {
	gorm.Model
	Name        string
	Addr        string
	User        string
	Password    string
	Fingerprint string
	PrivKey     *Key
}

type User struct {
	gorm.Model
	Keys []Key
}

func (u *User) Name() string {
	return "anonymous"
}

func dbInit(db *gorm.DB) error {
	db.AutoMigrate(&User{})
	db.AutoMigrate(&Key{})
	db.AutoMigrate(&Host{})
	return nil
}

func dbDemo(db *gorm.DB) error {
	var host1, host2, host3 Host
	db.FirstOrCreate(&host1, &Host{Name: "sdf", Addr: "sdf.org:22", User: "new"})
	db.FirstOrCreate(&host2, &Host{Name: "whoami", Addr: "whoami.filippo.io:22", User: "test"})
	db.FirstOrCreate(&host3, &Host{Name: "ssh-chat", Addr: "chat.shazow.net:22", User: "test", Fingerprint: "MD5:e5:d5:d1:75:90:38:42:f6:c7:03:d7:d0:56:7d:6a:db"})
	return nil
}
