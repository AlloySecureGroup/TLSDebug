package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShellQuote(t *testing.T) {
	actual := shellQuote("/tmp/it's here/ca.crt")
	expected := "'/tmp/it'\"'\"'s here/ca.crt'"
	if actual != expected {
		t.Fatalf("shellQuote() = %q, want %q", actual, expected)
	}
}

func TestResolveProxyBinaryUsesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-proxy")
	if err := os.WriteFile(path, []byte("proxy"), 0700); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	resolved, err := app.resolveProxyBinary(Settings{ProxyBinary: path})
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("resolveProxyBinary() = %q, want %q", resolved, path)
	}
}

func TestUpdateSettingsRejectsInvalidPorts(t *testing.T) {
	app := NewApp()
	_, err := app.UpdateSettings(Settings{
		ProxyPort:   0,
		MonitorPort: 4040,
		DataDir:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("UpdateSettings() accepted an invalid proxy port")
	}
}

func TestCopyRootCAUsesValidRootPair(t *testing.T) {
	root := t.TempDir()
	dataDir := t.TempDir()
	certPEM, keyPEM := createTestCA(t)
	if err := os.WriteFile(filepath.Join(root, "proxy-ca.crt"), certPEM, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proxy-ca.key"), keyPEM, 0600); err != nil {
		t.Fatal(err)
	}

	copied, err := copyRootCA(root, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !copied {
		t.Fatal("copyRootCA() did not detect the root CA pair")
	}
	actualCert, err := os.ReadFile(filepath.Join(dataDir, "proxy-ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	actualKey, err := os.ReadFile(filepath.Join(dataDir, "proxy-ca.key"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actualCert, certPEM) || !bytes.Equal(actualKey, keyPEM) {
		t.Fatal("copyRootCA() did not preserve the source CA pair")
	}
}

func TestCopyRootCARejectsIncompletePair(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "proxy-ca.crt"), []byte("certificate"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := copyRootCA(root, t.TempDir()); err == nil {
		t.Fatal("copyRootCA() accepted a certificate without its private key")
	}
}

func createTestCA(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "TLSDebug Test Root"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}
