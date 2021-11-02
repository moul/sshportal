package crypto // import "moul.io/sshportal/pkg/crypto"

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	"moul.io/sshportal/pkg/dbmodels"
)

func NewSSHKey(keyType string, length uint) (*dbmodels.SSHKey, error) {
	key := dbmodels.SSHKey{
		Type:   keyType,
		Length: length,
	}

	// generate the private key
	var err error
	var pemKey *pem.Block
	var publicKey gossh.PublicKey
	switch keyType {
	case "rsa":
		pemKey, publicKey, err = NewRSAKey(length)
	case "ecdsa":
		pemKey, publicKey, err = NewECDSAKey(length)
	case "ed25519":
		pemKey, publicKey, err = NewEd25519Key()
	default:
		return nil, fmt.Errorf("key type not supported: %q, supported types are: rsa, ecdsa, ed25519", key.Type)
	}
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBufferString("")
	if err = pem.Encode(buf, pemKey); err != nil {
		return nil, err
	}
	key.PrivKey = buf.String()

	// generate authorized-key formatted pubkey output
	key.PubKey = strings.TrimSpace(string(gossh.MarshalAuthorizedKey(publicKey)))

	return &key, nil
}

func NewRSAKey(length uint) (*pem.Block, gossh.PublicKey, error) {
	if length < 1024 || length > 16384 {
		return nil, nil, fmt.Errorf("key length not supported: %d, supported values are between 1024 and 16384", length)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, int(length))
	if err != nil {
		return nil, nil, err
	}
	// convert priv key to x509 format
	pemKey := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	publicKey, err := gossh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return pemKey, publicKey, err
}

func NewECDSAKey(length uint) (*pem.Block, gossh.PublicKey, error) {
	var curve elliptic.Curve
	switch length {
	case 256:
		curve = elliptic.P256()
	case 384:
		curve = elliptic.P384()
	case 521:
		curve = elliptic.P521()
	default:
		return nil, nil, fmt.Errorf("key length not supported: %d, supported values are 256, 384, 521", length)
	}
	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	// convert priv key to x509 format
	marshaledKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	pemKey := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: marshaledKey,
	}
	if err != nil {
		return nil, nil, err
	}
	publicKey, err := gossh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return pemKey, publicKey, err
}

func NewEd25519Key() (*pem.Block, gossh.PublicKey, error) {
	publicKeyEd25519, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	// convert priv key to x509 format
	marshaledKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	pemKey := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: marshaledKey,
	}
	if err != nil {
		return nil, nil, err
	}
	publicKey, err := gossh.NewPublicKey(publicKeyEd25519)
	if err != nil {
		return nil, nil, err
	}
	return pemKey, publicKey, err
}

func ImportSSHKey(keyValue string) (*dbmodels.SSHKey, error) {
	key := dbmodels.SSHKey{
		Type: "rsa",
	}

	parsedKey, err := gossh.ParseRawPrivateKey([]byte(keyValue))
	if err != nil {
		return nil, err
	}
	var privateKey *rsa.PrivateKey
	var ok bool
	if privateKey, ok = parsedKey.(*rsa.PrivateKey); !ok {
		return nil, errors.New("key type not supported")
	}
	key.Length = uint(privateKey.PublicKey.N.BitLen())
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
	return string(ciphertext), nil
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

func HostEncrypt(aesKey string, host *dbmodels.Host) (err error) {
	if aesKey == "" {
		return nil
	}
	if host.Password != "" {
		host.Password, err = encrypt([]byte(aesKey), host.Password)
	}
	return
}
func HostDecrypt(aesKey string, host *dbmodels.Host) {
	if aesKey == "" {
		return
	}
	if host.Password != "" {
		host.Password = safeDecrypt([]byte(aesKey), host.Password)
	}
}

func SSHKeyEncrypt(aesKey string, key *dbmodels.SSHKey) (err error) {
	if aesKey == "" {
		return nil
	}
	key.PrivKey, err = encrypt([]byte(aesKey), key.PrivKey)
	return
}
func SSHKeyDecrypt(aesKey string, key *dbmodels.SSHKey) {
	if aesKey == "" {
		return
	}
	key.PrivKey = safeDecrypt([]byte(aesKey), key.PrivKey)
}
