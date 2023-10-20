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
	return tlsConfig(serverTLSConfig, cfg)
}

func ClientTLSConfig(cfg *viper.Viper) (config *tls.Config, err error) {
	return tlsConfig(clientTLSConfig, cfg)
}

func tlsConfig(typ tlsConfigType, opts *viper.Viper) (config *tls.Config, err error) {

	flagOpts := TLS{}
	if err = opts.Unmarshal(&flagOpts); err != nil {
		return
	}

	config = new(tls.Config)

	if flagOpts.CertificateFile != "" {
		var crt, key []byte
		if crt, err = os.ReadFile(flagOpts.CertificateFile); err != nil {
			config = nil
			return
		}
		if flagOpts.KeyFile == "" {
			config = nil
			err = NewError("key file is required if certificate file is defined", 2)
			return
		} else if key, err = os.ReadFile(flagOpts.KeyFile); err != nil {
			config = nil
			err = WrapError(err, 2)
			return
		}

		if certificate, e := tls.X509KeyPair(crt, key); e == nil {
			config.Certificates = []tls.Certificate{certificate}
		} else {
			config = nil
			err = WrapError(e, 2)
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
				config = nil
				err = NewError("invalid certificate: "+c, 2)
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
