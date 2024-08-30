/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package grpctransport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These certs are not real, and not used for another other than UTs.

var (
	VALID_CERT = `-----BEGIN CERTIFICATE-----
MIIEYDCCAkigAwIBAgIUSq1CnHOv+627dDDGZXUDuH9JVxAwDQYJKoZIhvcNAQEL
BQAwSzELMAkGA1UEBhMCVVMxEzARBgNVBAgMClNvbWUtU3RhdGUxFzAVBgNVBAoM
DkR1bmRlciBNaWZmbGluMQ4wDAYDVQQLDAVTYWxlczAeFw0yNDA2MTgwOTMyMDJa
Fw0yNTA2MTgwOTMyMDJaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0
YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQCmbzKxmEKmVI22503kNuQv1GTSXUpBNSkE
CaBTLBuwOG4rDfp8pwMhrlcm2563lm6cPB0WOLQ8+GS84vSiXq68M1p2etQvbbIw
QKIelqzbC048kM1z/6LOAvXSf4NfEmlSR9L754qBY34ZmsMaegPP9iPFrjP7gx19
Oqt19+CLVHIA+b+HlwIDgUEcWs900rYCUwoaXsCRCeiZzDAGFGEYioqt7lepe1o/
6aHNrspKEXzfYUdHTa0q5br1jqa35fiLjNv1FV5i1I9diNwVJIVBG88ZsAo0PaD7
FrCN2aYZpopHzd8naPOpu9TD9q35fNuFeSd2BbFA5JKn9qaIz7d7AgMBAAGjQjBA
MB0GA1UdDgQWBBSlCLhFuf5bsLNQ5K3QCzOAIX5QQzAfBgNVHSMEGDAWgBRr+5Hs
UjAV5zjdJu3IGLuQHydmrjANBgkqhkiG9w0BAQsFAAOCAgEAQVc0G8t9gVt3y0zM
ECGF+7gXRaz5uBDDCyl0vVnljYpcuJCsJejy52euzE7WwI+tFbp0df13z6xmhj9C
3n3hiFXgOUkyOWiu22Dqq6fEDhcCDgGukSniuLKC01FNybS7RUnLv21PpYcYUD4n
E+sieJmW5/1VHRT7imINJGL2DeY0hfNNz1HTBD5ISG9I6frTelbPekyK9kBHFnSg
ksIR9nh9Z9NhXj8nVjANyNUjBMQULQKIwvKhPV80vrpNsXfmFJEGuEUz8i2Ku0z3
y++5qFTC3ADh9r6dyQzti9N+PFyjXmutj8MwJrc2oV09272kJXiq1f3DWT6iZyxU
F1igEuBcDqRLBD2w/XOwiWdw7egleH9ayn8oqAmZigvkRTOLX1irb4IinyRZnRJI
kvoWmnCfdu4xuTpsAOYIde19P9upeOs6ikwQejhvqSxuXwmqwtNz7Ww6SgbGPPit
Yc3ykcSFHcfM+RReXkZfLW2q39bK7SxoKHjHa49jT9/Fo293nFGVyikUngIoc7Nv
5jzwMYd3W2i75FPY4T5nGUuox000ByN0uPe85uakxyz/7TqB5dhuZraRQP8xO/8x
HtlF1HQ3Xl8BSiJxFucV8GBU9VkGv/mP2kqDGed3mfWcEOmPGMnPRq/bKUKVDgOe
ji62H8shNiVprlf5kKYXCCG0V28=
-----END CERTIFICATE-----`
	VALID_KEY = `-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCmbzKxmEKmVI22
503kNuQv1GTSXUpBNSkECaBTLBuwOG4rDfp8pwMhrlcm2563lm6cPB0WOLQ8+GS8
4vSiXq68M1p2etQvbbIwQKIelqzbC048kM1z/6LOAvXSf4NfEmlSR9L754qBY34Z
msMaegPP9iPFrjP7gx19Oqt19+CLVHIA+b+HlwIDgUEcWs900rYCUwoaXsCRCeiZ
zDAGFGEYioqt7lepe1o/6aHNrspKEXzfYUdHTa0q5br1jqa35fiLjNv1FV5i1I9d
iNwVJIVBG88ZsAo0PaD7FrCN2aYZpopHzd8naPOpu9TD9q35fNuFeSd2BbFA5JKn
9qaIz7d7AgMBAAECggEAEIXRNbdPb+gh/3/xXXrOPCoCQ64WQpNOpJjODU5qMJw8
N6hWGeI8ry8kdiDvHfu1tGtKr0Y8LSIIYjDH0jIYd7+Yt1K/Vja2LhB5575+n5T8
6d9YPi/+j3l/bQEoChwE+lxTXPBiHDFD4gfZXvtbqIgfigRJeH/OQXyTFrg1LFpc
kWafOCclW4X56Yi6o/Z/B/r4559R6zRMPvMTgCP7rJLM8AzvZBhKdO+IQy7CWO3N
9qJ8Ruwqg0roW7y5Igem1Mkt9rr1m7Y7A8FOng5O1prTmy3XLISIflLml2sneOcU
gAn4zzemosIzi/e3OEOMS4iZImrUUprVzi8ewcM6UQKBgQDb45qZVsTmp8T/zPrk
ReWCZL7fdtgFEPaEzW2R7ISFmiIRJ16HSv7/fwf9KGbaiN79IKtYL1yc8kIgOrOl
lg3kADl/SbcCULdaml25eo7FAnNqVS9rC7bTMqpSBwQKIfbHsSHMTAyrDCcpf2m0
8kgQj8flagDd5i18c9V3jrchKwKBgQDBxEuJULyXDZC/ASqYo2VNii4wgR1kPXws
clY+wLrCgYs0blyUW1MC96CFq71arhnuTNQaiwjiVl58Jr/EoeHN83vppCNlxQoC
Qr1KA/rHfqFGze2eljf6SKP/HC40Noqy5BAUrCo7ri9lkg3zzWT/Wp9V0y0zGG9d
4WlUPlF68QKBgEzwF4D9tkmKfEiQ3VyQBF0aiRrBh5ZGOCuBgYwkeL4Sj0b3QoEO
Qaj+Nrpir5JZf8SL8MgbUklsDq0ePQt8NIIJfhTGoy1BNtaR2qdydZADaoTTJ5NJ
9v9w491trQEqnyGCHfRQy8Hxr6Y6ea8hcwuwc14XOBRsXJAGK7P4kKUfAoGAKkO9
0jLZH/2mf/MqLmcdlp84Wf4fwhijODKsWz3AvuJw0bvs17Nf1PrceauvPtNQ9qit
byi6hFwgeeYd2C8TSM/+TEUwL2eeOkYCcd2SWudlGEOcvAW6Kg6kLuYfseXftQuH
8mpnP+NLlVUZU/+OMyC6Noc8ibOfvxOG3QhGz5ECgYAL1faC+TDqxh5Wja4r8zql
LKoOrb1/LPEb0TJfeKXin6QiKGkXCkNE9ZwxBlU4oen012fWyWBjhiv0eoRXJz5P
9qwgyOvPE/imOkgjrB9L5cFRJF0Nm2p+kLCzjEO2YIE2a2Yv1JMiRG7CYIwc1qrk
0d8wt4Z7zV78vbfwRxatdg==
-----END PRIVATE KEY-----`
INVALID_CERT = `-----BEGIN CERTIFICATE-----
MIIEYDCCAkigAwIBAgIUSq1CnHOv+627dDDGZXUDuH9JVxAwDQYJKoZIhvcNAQEL
-----END CERTIFICATE-----`
	INVALID_KEY  = `-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCmbzKxmEKmVI22
-----END PRIVATE KEY-----`
)

func TestProcessGRPCConfigOK(t *testing.T) {
	validConfiguration := &UnprocessedGRPCConfig{
		ServerCertificate: &VALID_CERT,
		ServerKey:         &VALID_KEY,
		ClientCertificate: &VALID_CERT,
		ClientKey:         &VALID_KEY,
		ExternalPort:      8080,
	}

	processedConfig, err := ProcessGRPCConfig(validConfiguration)
	assert.NoError(t, err)

	assert.NotNil(t, processedConfig.ServerCertificate)
	assert.NotNil(t, processedConfig.ClientCertificate)
	assert.Equal(t, 8080, processedConfig.ExternalPort)
}

func TestProcessGRPCConfigBadCertificates(t *testing.T) {
	validConfiguration := &UnprocessedGRPCConfig{
		ServerCertificate: &INVALID_CERT,
		ServerKey:         &VALID_KEY,
		ClientCertificate: &INVALID_CERT,
		ClientKey:         &VALID_KEY,
		ExternalPort:      8080,
	}

	_, err := ProcessGRPCConfig(validConfiguration)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "malformed certificate")

	validConfiguration = &UnprocessedGRPCConfig{
		ServerCertificate: &VALID_CERT,
		ServerKey:         &VALID_KEY,
		ClientCertificate: &INVALID_CERT,
		ClientKey:         &VALID_KEY,
		ExternalPort:      8080,
	}

	_, err = ProcessGRPCConfig(validConfiguration)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "malformed certificate")
}

func TestProcessGRPCConfigNil(t *testing.T) {
	_, err := ProcessGRPCConfig(nil)
	assert.Contains(t, err.Error(), "no unprocessed config provided")
}
