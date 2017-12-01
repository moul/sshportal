package main

import (
	"bytes"
	"fmt"
	"log"
	"net"

	"github.com/jinzhu/gorm"
	gossh "golang.org/x/crypto/ssh"
)

type dynamicHostKey struct {
	db   *gorm.DB
	host *Host
}

func (d *dynamicHostKey) check(hostname string, remote net.Addr, key gossh.PublicKey) error {
	if len(d.host.HostKey) == 0 {
		log.Println("Discovering host fingerprint...")
		return d.db.Model(d.host).Update("HostKey", key.Marshal()).Error
	}

	if !bytes.Equal(d.host.HostKey, key.Marshal()) {
		return fmt.Errorf("ssh: host key mismatch")
	}
	return nil
}

// DynamicHostKey returns a function for use in
// ClientConfig.HostKeyCallback to dynamically learn or accept host key.
func DynamicHostKey(db *gorm.DB, host *Host) gossh.HostKeyCallback {
	// FIXME: forward interactively the host key checking
	hk := &dynamicHostKey{db, host}
	return hk.check
}
