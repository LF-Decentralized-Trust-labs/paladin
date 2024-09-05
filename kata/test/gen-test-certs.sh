#!/bin/bash

rm -rf ca1/
rm -rf ca2/
rm -rf ca3/

# Check if OpenSSL is installed
if ! command -v openssl &> /dev/null
then
    echo "OpenSSL could not be found. Please install it and run the script again."
    exit
fi

# Function to create a CA
create_ca() {
    local ca_index=$1
    mkdir -p ca$ca_index
    cd ca$ca_index
    
    # Generate private key for CA
    openssl genpkey -algorithm RSA -out ca.key -aes256 -pass pass:password
    
    # Generate self-signed certificate for CA, valid for 10 years
    openssl req -x509 -new -key ca.key -days 3650 -out ca.crt -subj "/C=US/ST=State/L=Locality/O=Organization/OU=OrgUnit/CN=localhost" -passin pass:password
    
    cd ..
}

# Function to create a client certificate signed by a specific CA
create_client_cert() {
    local ca_index=$1
    local client_index=$2
    mkdir -p ca$ca_index/clients
    cd ca$ca_index/clients
    
    # Generate private key for client
    openssl genpkey -algorithm RSA -out client$client_index.key
    
    # Create a config file for the extensions
    cat > client$client_index.ext <<-EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
[req_distinguished_name]
[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = localhost
EOF
    
    # Generate CSR (Certificate Signing Request) for client
    openssl req -new -key client$client_index.key -out client$client_index.csr -subj "/C=US/ST=State/L=Locality/O=Organization/OU=OrgUnit/CN=localhost" -config client$client_index.ext
    
    # Generate client certificate signed by CA, valid for 10 years
    openssl x509 -req -in client$client_index.csr -CA ../ca.crt -CAkey ../ca.key -CAcreateserial -out client$client_index.crt -days 3650 -extfile client$client_index.ext -extensions v3_req -passin pass:password
    
    cd ../..
}

num_cas=3
num_clients=2

# Create CAs
for ((i=1; i<=num_cas; i++))
do
    create_ca $i
done

# Create client certificates for each CA
for ((i=1; i<=num_cas; i++))
do
    for ((j=1; j<=num_clients; j++))
    do
        create_client_cert $i $j
    done
done

echo "CAs and client certificates created successfully."
