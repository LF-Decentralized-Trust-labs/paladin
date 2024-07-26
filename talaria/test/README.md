## Component Test Certificates

In this folder are certificates used for the component test of the gRPC plugin (since it uses mTLS), the certificates
here are generated specifically for this test are not used elsewhere. Each diretory contains a single leaf certificate
and the CA that was used to generate it.

The CA certificates in this directory are secured with the password 'password', and all of the certificates expire after
10 years.