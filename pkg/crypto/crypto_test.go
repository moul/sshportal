package crypto

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestImportSSHKeyRoundTrip(t *testing.T) {
	tests := []struct {
		keyType string
		length  uint
	}{
		{keyType: "rsa", length: 2048},
		{keyType: "ecdsa", length: 256},
		{keyType: "ed25519", length: 1},
	}
	for _, tt := range tests {
		t.Run(tt.keyType, func(t *testing.T) {
			original, err := NewSSHKey(tt.keyType, tt.length)
			if err != nil {
				t.Fatal(err)
			}
			checkImportedSSHKey(t, original.PrivKey, original.PubKey, tt.keyType, tt.length)
		})
	}
}

func TestImportOpenSSHEd25519Key(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is unavailable")
	}
	dir, err := ioutil.TempDir("", "sshportal-key-import")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	keyPath := filepath.Join(dir, "id_ed25519")
	if output, err := exec.Command(sshKeygen, "-q", "-t", "ed25519", "-N", "", "-f", keyPath).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, output)
	}
	privateKey, err := ioutil.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ioutil.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	checkImportedSSHKey(t, string(privateKey), string(publicKey), "ed25519", 1)
}

func checkImportedSSHKey(t *testing.T, originalPrivate, originalPublic, keyType string, length uint) {
	t.Helper()
	wantPublic, _, _, _, err := gossh.ParseAuthorizedKey([]byte(originalPublic))
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportSSHKey(originalPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Type != keyType || imported.Length != length {
		t.Fatalf("imported type/length = %s/%d, want %s/%d", imported.Type, imported.Length, keyType, length)
	}
	signer, err := gossh.ParsePrivateKey([]byte(imported.PrivKey))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(wantPublic))); imported.PubKey != got {
		t.Fatal("imported public key does not match original")
	}
	if got := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(signer.PublicKey()))); imported.PubKey != got {
		t.Fatal("stored private key does not match its public key")
	}
}
