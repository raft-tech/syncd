package helpers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	mrand "math/rand"
	"time"
)

func SelfSignedCertificate(cn string, sans ...string) (*tls.Certificate, error) {

	var key *ecdsa.PrivateKey
	if k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader); e == nil {
		key = k
	} else {
		return nil, e
	}

	crt := &x509.Certificate{
		SerialNumber: big.NewInt(int64(mrand.Int())),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:              sans,
	}

	if b, e := x509.CreateCertificate(rand.Reader, crt, crt, &key.PublicKey, key); e == nil {
		if crt, e = x509.ParseCertificate(b); e != nil {
			return nil, e
		}
		return &tls.Certificate{
			Certificate: [][]byte{b},
			PrivateKey:  key,
			Leaf:        crt,
		}, nil
	} else {
		return nil, e
	}
}
