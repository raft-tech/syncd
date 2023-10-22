package cmd_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/syncd/internal/helpers"
)

var clientTLS *tls.Config

var serverConfig = []byte(`
server:
  listen: ":8080"
  metrics:
    listen: ":8081"
  tls:
    crt: build/server.crt
    key: build/server.key
    ca:
      - build/client.crt
  auth:
    preSharedKey:
      value: cookiecookiecookiecookie
  models:
    artists:
      filters:
        - key: name
          operator: In
          value:
            - John Lennon
            - Paul McCartney
            - Ringo Starr
            - George Harrison
    performers:
      filters:
        - key: label
          operator: Equal
          value: VeeJay
    songs:
      filters:
        - key: publisher
          operator: Equal
          value: VeeJay
graph:
  source:
    postgres:
      syncTable:
        name: syncd.sync
      sequenceTable:
        name: syncd.sync_seq
      connection:
        value: postgres://testing
  models:
    artists:
      table: syncd.artists
      key: id
      version: version
    songs:
      table: syncd.songs
      key: id
      version: version
    performers:
      table: syncd.performers
      key: id
      version: version
      children:
        artists:
          table: syncd.performer_artists
          key: performer
        performances:
          table: syncd.performances
          key: performer
          sequence: sequence
`)

var _ = BeforeSuite(func() {

	Expect(os.MkdirAll("build", 0755)).NotTo(HaveOccurred())

	// Config file
	Expect(os.WriteFile("build/config.yaml", serverConfig, 0644)).NotTo(HaveOccurred())

	// Server Cert
	serverCA := x509.NewCertPool()
	if crt, err := helpers.SelfSignedCertificate("syncd"); err == nil {
		serverCA.AddCert(crt.Leaf)
		buf := new(bytes.Buffer)
		for _, b := range crt.Certificate {
			block := pem.Block{
				Type:  "CERTIFICATE",
				Bytes: b,
			}
			Expect(pem.Encode(buf, &block)).To(Succeed())
		}
		Expect(os.WriteFile("build/server.crt", buf.Bytes(), 0644)).NotTo(HaveOccurred())
		buf.Reset()
		var b []byte
		switch crt.PrivateKey.(type) {
		case *ecdsa.PrivateKey:
			b, err = x509.MarshalECPrivateKey(crt.PrivateKey.(*ecdsa.PrivateKey))
			Expect(err).NotTo(HaveOccurred())
		default:
			Fail("unrecognized private key type: " + reflect.TypeOf(crt.PrivateKey).String())
			return
		}
		Expect(pem.Encode(buf, &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: b,
		})).To(Succeed())
		Expect(os.WriteFile("build/server.key", buf.Bytes(), 0640)).NotTo(HaveOccurred())
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	// Client Cert
	if crt, err := helpers.SelfSignedCertificate("syncd"); err == nil {

		clientTLS = &tls.Config{
			Certificates: []tls.Certificate{*crt},
			RootCAs:      serverCA,
		}

		// Write the certificate to a file for the server to use as a CA pool
		buf := new(bytes.Buffer)
		for _, b := range crt.Certificate {
			block := pem.Block{
				Type:  "CERTIFICATE",
				Bytes: b,
			}
			Expect(pem.Encode(buf, &block)).To(Succeed())
		}
		Expect(os.WriteFile("build/client.crt", buf.Bytes(), 0644)).NotTo(HaveOccurred())
		buf.Reset()
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
})

var _ = AfterSuite(func() {
	Expect(os.RemoveAll("build")).To(Succeed())
})
