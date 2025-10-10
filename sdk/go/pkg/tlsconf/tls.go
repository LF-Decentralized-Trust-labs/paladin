// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tlsconf

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"os"
	"regexp"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

type TLSType string

const (
	ServerType TLSType = "server"
	ClientType TLSType = "client"
)

type TLSConfigDetailed struct {
	TLSConfig   *tls.Config
	Certificate *tls.Certificate
}

func BuildTLSConfig(ctx context.Context, config *pldconf.TLSConfig, tlsType TLSType) (*tls.Config, error) {
	conf, err := BuildTLSConfigExt(ctx, config, tlsType)
	if err != nil || conf == nil {
		return nil, err
	}
	return conf.TLSConfig, nil
}

func BuildTLSConfigExt(ctx context.Context, config *pldconf.TLSConfig, tlsType TLSType) (*TLSConfigDetailed, error) {
	if !config.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		VerifyPeerCertificate: func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
				cert := verifiedChains[0][0]
				log.L(ctx).Debugf("Client certificate provided Subject=%s Issuer=%s Expiry=%s", cert.Subject, cert.Issuer, cert.NotAfter)
			} else {
				log.L(ctx).Debugf("Client certificate unverified")
			}
			return nil
		},
	}
	detail := &TLSConfigDetailed{TLSConfig: tlsConfig}

	var err error
	// Support custom CA file
	var rootCAs *x509.CertPool
	switch {
	case config.CAFile != "":
		rootCAs = x509.NewCertPool()
		var caBytes []byte
		caBytes, err = os.ReadFile(config.CAFile)
		if err == nil {
			ok := rootCAs.AppendCertsFromPEM(caBytes)
			if !ok {
				err = i18n.NewError(ctx, pldmsgs.MsgTLSInvalidCAFile)
			}
		}
	case config.CA != "":
		rootCAs = x509.NewCertPool()
		ok := rootCAs.AppendCertsFromPEM([]byte(config.CA))
		if !ok {
			err = i18n.NewError(ctx, pldmsgs.MsgTLSInvalidCAFile)
		}
	default:
		rootCAs, err = x509.SystemCertPool()
	}

	if err != nil {
		return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTLSConfigFailed)
	}

	tlsConfig.RootCAs = rootCAs

	// For mTLS we need both the cert and key
	if config.CertFile != "" && config.KeyFile != "" {
		// Read the key pair to create certificate
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTLSInvalidKeyPairFiles)
		}
		detail.Certificate = &cert
	} else if config.Cert != "" && config.Key != "" {
		cert, err := tls.X509KeyPair([]byte(config.Cert), []byte(config.Key))
		if err != nil {
			return nil, i18n.WrapError(ctx, err, pldmsgs.MsgTLSInvalidKeyPairFiles)
		}
		detail.Certificate = &cert
	}
	if detail.Certificate != nil {
		configuredCert := detail.Certificate
		// Rather than letting Golang pick a certificate it thinks matches from the list of one,
		// we directly supply it the one we have in all cases.
		tlsConfig.GetClientCertificate = func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			log.L(ctx).Debugf("Supplying client certificate")
			return configuredCert, nil
		}
		tlsConfig.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			log.L(ctx).Debugf("Supplying server certificate")
			return configuredCert, nil
		}
	}

	if tlsType == ServerType {

		// Support client auth
		tlsConfig.ClientAuth = tls.NoClientCert
		if config.ClientAuth {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert

			// Used to verify a client certificate by the policy in ClientAuth.
			tlsConfig.ClientCAs = rootCAs
		}

	}

	if len(config.RequiredDNAttributes) > 0 {
		if tlsConfig.VerifyPeerCertificate, err = buildDNValidator(ctx, config.RequiredDNAttributes); err != nil {
			return nil, err
		}
	}

	tlsConfig.InsecureSkipVerify = config.InsecureSkipHostVerify

	return detail, nil

}

var SubjectDNKnownAttributes = map[string]func(pkix.Name) []string{
	"C": func(n pkix.Name) []string {
		return n.Country
	},
	"O": func(n pkix.Name) []string {
		return n.Organization
	},
	"OU": func(n pkix.Name) []string {
		return n.OrganizationalUnit
	},
	"CN": func(n pkix.Name) []string {
		if n.CommonName == "" {
			return []string{}
		}
		return []string{n.CommonName}
	},
	"SERIALNUMBER": func(n pkix.Name) []string {
		if n.SerialNumber == "" {
			return []string{}
		}
		return []string{n.SerialNumber}
	},
	"L": func(n pkix.Name) []string {
		return n.Locality
	},
	"ST": func(n pkix.Name) []string {
		return n.Province
	},
	"STREET": func(n pkix.Name) []string {
		return n.StreetAddress
	},
	"POSTALCODE": func(n pkix.Name) []string {
		return n.PostalCode
	},
}

func buildDNValidator(ctx context.Context, requiredDNAttributes map[string]string) (func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error, error) {

	validators := make(map[string]*regexp.Regexp)
	for attr, validatorString := range requiredDNAttributes {
		attr = strings.ToUpper(attr)
		if _, knownAttr := SubjectDNKnownAttributes[attr]; !knownAttr {
			return nil, i18n.NewError(ctx, pldmsgs.MsgTLSInvalidTLSDnMatcherAttr, attr)
		}
		// Ensure full string match with all regexp
		validatorString = "^" + strings.TrimSuffix(strings.TrimPrefix(validatorString, "^"), "$") + "$"
		validator, err := regexp.Compile(validatorString)
		if err != nil {
			return nil, i18n.NewError(ctx, pldmsgs.MsgTLSInvalidTLSDnMatcherRegexp, validatorString, attr, err)
		}
		validators[attr] = validator
	}
	return func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(verifiedChains) == 0 {
			log.L(ctx).Errorf("Failed TLS DN check: Nil cert chain")
			return i18n.NewError(ctx, pldmsgs.MsgTLSInvalidTLSDnChain)
		}
		for iChain, chain := range verifiedChains {
			if len(chain) == 0 {
				log.L(ctx).Errorf("Failed TLS DN check: Empty cert chain %d", iChain)
				return i18n.NewError(ctx, pldmsgs.MsgTLSInvalidTLSDnChain)
			}
			// We get a chain of one or more certificates, leaf first.
			// Only check the leaf.
			cert := chain[0]
			log.L(ctx).Debugf("Performing TLS DN check on '%s'", cert.Subject)
			for attr, validator := range validators {
				matched := false
				values := SubjectDNKnownAttributes[attr](cert.Subject) // Note check above makes this safe
				for _, value := range values {
					matched = matched || validator.MatchString(value)
				}
				if !matched {
					log.L(ctx).Errorf("Failed TLS DN check: Does not match %s =~ /%s/", attr, validator.String())
					return i18n.NewError(ctx, pldmsgs.MsgTLSInvalidTLSDnMismatch)
				}
			}
		}
		return nil
	}, nil

}
