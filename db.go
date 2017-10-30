package main

import "github.com/jinzhu/gorm"

type Key struct {
	gorm.Model
}

type Host struct {
	gorm.Model
	Name     string
	Addr     string
	User     string
	Password string
	Key      Key
}

type User struct {
	gorm.Model
	Keys []Key
}

func dbInit(db *gorm.DB) error {
	db.LogMode(true)
	db.AutoMigrate(&User{})
	db.AutoMigrate(&Key{})
	db.AutoMigrate(&Host{})
	return nil
}

func dbDemo(db *gorm.DB) error {
	var host Host
	db.FirstOrCreate(&host, &Host{Name: "sdf", Addr: "sdf.org:22", User: "new"})
	return nil
}
