package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

func NewSSHKey(keyType string, length uint) (*SSHKey, error) {
	key := SSHKey{
		Type:   keyType,
		Length: length,
	}

	// generate the private key
	if keyType != "rsa" {
		return nil, fmt.Errorf("key type not supported: %q", key.Type)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// convert priv key to x509 format
	var pemKey = &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	buf := bytes.NewBufferString("")
	if err = pem.Encode(buf, pemKey); err != nil {
		return nil, err
	}
	key.PrivKey = buf.String()

	// generte authorized-key formatted pubkey output
	pub, err := gossh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	key.PubKey = strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub)))

	return &key, nil
}
