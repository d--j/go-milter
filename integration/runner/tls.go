package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path"
	"time"
)

func GenCert(host string, outDir string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(time.Hour * 24),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}
	b := &bytes.Buffer{}
	err = pem.Encode(b, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return fmt.Errorf("failed to encode certificate: %w", err)
	}
	f, err := os.OpenFile(path.Join(outDir, "cert.pem"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create certificate file: %w", err)
	}
	_, err = f.Write(b.Bytes())
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}
	b.Reset()
	err = pem.Encode(b, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err != nil {
		return fmt.Errorf("failed to encode key: %w", err)
	}
	f, err = os.OpenFile(path.Join(outDir, "key.pem"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create certificate file: %w", err)
	}
	_, err = f.Write(b.Bytes())
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}
	return nil
}
