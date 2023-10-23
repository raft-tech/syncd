/*
 * Copyright (c) 2023. Raft, LLC
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

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
