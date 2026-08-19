package transport_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qutaq/gophkeeper/internal/client/transport"
	"github.com/qutaq/gophkeeper/internal/config"
)

func TestDialTLS(t *testing.T) {
	t.Parallel()

	// Dial does not connect until RPC; NewClient should succeed with TLS creds.
	conn, err := transport.Dial(config.Client{
		ServerAddress: "127.0.0.1:1",
		TLS:           config.ClientTLS{InsecureSkipVerify: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}

func TestDialTLSWithCA(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	writeSelfSignedCA(t, caFile)

	conn, err := transport.Dial(config.Client{
		ServerAddress: "127.0.0.1:1",
		TLS: config.ClientTLS{
			CAFile: caFile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}

func TestDialTLSInvalidCA(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(path, []byte("not-a-cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := transport.Dial(config.Client{
		ServerAddress: "127.0.0.1:1",
		TLS:           config.ClientTLS{CAFile: path},
	})
	if err == nil {
		t.Fatal("expected invalid ca error")
	}
}

func writeSelfSignedCA(t *testing.T, path string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
}
