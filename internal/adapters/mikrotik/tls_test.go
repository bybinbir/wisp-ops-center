package mikrotik

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

// makeTestCAPEM, in-process üretilmiş geçerli bir self-signed sertifikayı
// PEM olarak döner. Test verisi için yeterli (BuildAPITLSConfig'in yalnızca
// AppendCertsFromPEM çağrısını ve handshake yapmadığını test ediyoruz).
func makeTestCAPEM(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "wisp-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestBuildAPITLSConfig_DefaultInsecure(t *testing.T) {
	c, err := BuildAPITLSConfig(Config{VerifyTLS: false})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !c.InsecureSkipVerify {
		t.Errorf("VerifyTLS=false should produce InsecureSkipVerify=true")
	}
	if c.MinVersion < 0x0303 {
		t.Errorf("MinVersion must be >= TLS1.2")
	}
}

func TestBuildAPITLSConfig_ServerNameOverrideAppliedWhenInsecure(t *testing.T) {
	c, err := BuildAPITLSConfig(Config{
		VerifyTLS:          false,
		ServerNameOverride: "router.lab",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.ServerName != "router.lab" {
		t.Errorf("server_name_override should still apply for SNI; got %q", c.ServerName)
	}
}

func TestBuildAPITLSConfig_VerifyTLSWithoutCA(t *testing.T) {
	c, err := BuildAPITLSConfig(Config{VerifyTLS: true})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.InsecureSkipVerify {
		t.Errorf("VerifyTLS=true must enable peer verification")
	}
	if c.RootCAs != nil {
		t.Errorf("expected nil RootCAs (system trust) when no CA provided")
	}
}

func TestBuildAPITLSConfig_VerifyTLSWithCustomCA(t *testing.T) {
	pem := makeTestCAPEM(t)
	c, err := BuildAPITLSConfig(Config{
		VerifyTLS:        true,
		CACertificatePEM: pem,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.RootCAs == nil {
		t.Errorf("expected RootCAs populated from PEM")
	}
}

func TestBuildAPITLSConfig_InvalidCA_FailsClosed(t *testing.T) {
	_, err := BuildAPITLSConfig(Config{
		VerifyTLS:        true,
		CACertificatePEM: "this-is-not-a-pem",
	})
	if !errors.Is(err, ErrInvalidCA) {
		t.Fatalf("expected ErrInvalidCA, got %v", err)
	}
}

func TestBuildAPITLSConfig_ServerNameOverrideAppliedWhenSecure(t *testing.T) {
	c, err := BuildAPITLSConfig(Config{
		VerifyTLS:          true,
		ServerNameOverride: "ap-north.example.com",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.ServerName != "ap-north.example.com" {
		t.Errorf("server_name_override must apply; got %q", c.ServerName)
	}
}
