package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
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

func encrypt(key []byte, text string) (string, error) {
	plaintext := []byte(text)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func decrypt(key []byte, cryptoText string) (string, error) {
	ciphertext, _ := base64.URLEncoding.DecodeString(cryptoText)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < aes.BlockSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	return fmt.Sprintf("%s", ciphertext), nil
}

func safeDecrypt(key []byte, cryptoText string) string {
	if len(key) == 0 {
		return cryptoText
	}
	out, err := decrypt(key, cryptoText)
	if err != nil {
		return cryptoText
	}
	return out
}

func HostEncrypt(aesKey string, host *Host) (err error) {
	if aesKey == "" {
		return nil
	}
	if host.Password != "" {
		host.Password, err = encrypt([]byte(aesKey), host.Password)
	}
	return
}
func HostDecrypt(aesKey string, host *Host) {
	if aesKey == "" {
		return
	}
	if host.Password != "" {
		host.Password = safeDecrypt([]byte(aesKey), host.Password)
	}
}

func SSHKeyEncrypt(aesKey string, key *SSHKey) (err error) {
	if aesKey == "" {
		return nil
	}
	key.PrivKey, err = encrypt([]byte(aesKey), key.PrivKey)
	return
}
func SSHKeyDecrypt(aesKey string, key *SSHKey) {
	if aesKey == "" {
		return
	}
	key.PrivKey = safeDecrypt([]byte(aesKey), key.PrivKey)
}
