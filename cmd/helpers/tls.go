package helpers

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/spf13/viper"
)

type tlsConfigType uint8

const (
	serverTLSConfig tlsConfigType = iota
	clientTLSConfig
)

func ServerTLSConfig(cfg *viper.Viper) (config *tls.Config, err error) {
	flagOpts := TLS{}
	if err = cfg.Unmarshal(&flagOpts); err != nil {
		return
	}
	return tlsConfig(serverTLSConfig, flagOpts)
}

func ClientTLSConfig(flagOpts TLS) (config *tls.Config, err error) {
	return tlsConfig(clientTLSConfig, flagOpts)
}

func tlsConfig(typ tlsConfigType, flagOpts TLS) (config *tls.Config, err error) {

	config = new(tls.Config)
	fail := func(e error) {
		config = nil
		if _, ok := e.(interface{ Code() int }); !ok {
			e = WrapError(e, 2)
		}
		err = e
		return
	}

	if flagOpts.CertificateFile != "" {
		var crt, key []byte
		if crt, err = os.ReadFile(flagOpts.CertificateFile); err != nil {
			fail(err)
			return
		}
		if flagOpts.KeyFile == "" {
			fail(NewError("key file is required if certificate file is defined", 2))
			return
		} else if key, err = os.ReadFile(flagOpts.KeyFile); err != nil {
			fail(err)
			return
		}

		if certificate, e := tls.X509KeyPair(crt, key); e == nil {
			config.Certificates = []tls.Certificate{certificate}
		} else {
			fail(e)
			return
		}
	}

	if len(flagOpts.CA) > 0 {
		pool := x509.NewCertPool()
		for _, c := range flagOpts.CA {
			var crt []byte
			if crt, err = os.ReadFile(c); err != nil {
				return
			}
			if !pool.AppendCertsFromPEM(crt) {
				fail(NewError("invalid certificate: "+c, 2))
				return
			}
		}
		switch typ {
		case serverTLSConfig:
			config.ClientCAs = pool
			config.ClientAuth = tls.RequireAndVerifyClientCert
		case clientTLSConfig:
			config.RootCAs = pool
		}
	} else if typ == clientTLSConfig {
		if pool, e := x509.SystemCertPool(); e == nil {
			config.RootCAs = pool
		}
	}

	return
}

type TLS struct {
	CA              []string
	CertificateFile string `mapstructure:"crt"`
	KeyFile         string `mapstructure:"key"`
}
