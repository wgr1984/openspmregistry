```
  ___                   ____  ____  __  __ ____            _     _              
 / _ \ _ __   ___ _ __ / ___||  _ \|  \/  |  _ \ ___  __ _(_)___| |_ _ __ _   _ 
| | | | '_ \ / _ \ '_ \\___ \| |_) | |\/| | |_) / _ \/ _` | / __| __| '__| | | |
| |_| | |_) |  __/ | | |___) |  __/| |  | |  _ <  __/ (_| | \__ \ |_| |  | |_| |
 \___/| .__/ \___|_| |_|____/|_|   |_|  |_|_| \_\___|\__, |_|___/\__|_|   \__, |
      |_|                                            |___/                |___/ 
```

Simple (using as much go std. lib and as less external dependencies as possible) implementation of Swift Package Manager Registry according to
https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/Registry.md

# Current Features
Basic Publishing and retrieval of swift packages
- ✔️ Listing / Browsing
- ✔️Retrieval of Packages
- ✔️ Publishing
  - ✔️ synchronous
  - ✔️ binary format
  - ✔️ Support Signatures
  - ✔️ support metadata block
- ✔️ Storage
  - ✔️ Filesystem
- ✔️ Docker Image sample

# How To Use
## Run server
fetch from repo
```
git clone https://github.com/wgr1984/openspmregistry.git
```
### Using docker image
create image
```
make
```
run image
```
docker run -p 8080:8080 -v ./:/data -i -t openspmregistry:latest
```
### From source
build / run

⚠️ check `server.yml` to e.g. adapt path and port
```
 go run main.go -tls=true -v
```

## Usage in SPM
### Create New Project
https://www.swift.org/documentation/package-manager/
e.g.
```
mkdir spm_test_lib
cd spm_test_lib
swift package init --type=library 
```
ensure spm registry is known and setup
e.g. `localhost` (be aware `swift package-registry` as for now accepts tls/ssl connections only)
```
swift package-registry set https://localhost:8080
```
⚠️ on local setup we need to make sure ssl cert is set too trusted on system level
```
swift package-registry set https://localhost:8080
```
### Publish
## Simple
```
swift package-registry publish test.TestLib 0.0.1
```
##Using metadata and signing
```
swift package-registry publish test.TestLib 0.0.1 --metadata-path package-metadata.json --signing-identity "[your identity]"
```
### package-metadata.json sample
```
{
  "author": {
    "name": "Wolfgang Reithmeier",
    "email": "w.reithmeier@gmail.com",
    "organization": {
      "name": "wgr1984"
    }
  },
  "description": "test spm app",
  "licenseURL": "https://github.com/wgr1984/spm_test/LCENCE.TXT",
  "readmeURL": "https://github.com/wgr1984/spm_test/README.md",
  "repositoryURLs": [
    "https://github.com/wgr1984/spm_test.git",
    "git@github.com:wgr1984/spm_test.git"
  ]
}
```
### How to create a signing identity
#### Create a CA
```
openssl genpkey -algorithm RSA  -outform der -out ca.key -pkeyopt rsa_keygen_bits:2048
openssl req -x509 -new -nodes -key ca.key -sha256 -days 365  -outform der -out ca.crt -subj "/C=US/ST=State/L=City/O=Organization/OU=OrgUnit/CN=RootCA"
```
#### Create a certificate
```
openssl ecparam -genkey -name prime256v1  -outform der -out ecdsa.key
openssl pkcs8 -topk8 -inform DER -outform PEM -in ecdsa.key -out ecdsa_temp.pem -nocrypt
openssl asn1parse -in ecdsa_temp.pem -out ecdsa.key.der -noout
KEY=$(openssl dgst -sha1 -binary ecdsa.key.der | xxd -p | tr -d '\n' | sed 's/\(..\)/\1 /g')
echo "Bag Attributes
    friendlyName: SPM_TEST
    localKeyID: $KEY
Key Attributes: <No Attributes>" > ecdsa.pem
cat ecdsa_temp.pem >> ecdsa.pem

openssl pkcs8 -topk8 -inform pem -outform der -in ecdsa.pem -out ecdsa_pkcs8.key -nocrypt
openssl req -new -key ecdsa.key -out ecdsa.csr -subj "/C=US/ST=State/L=City/O=Organization/OU=OrgUnit/CN=CodeSigningCert"

echo "[ v3_code_sign ]
keyUsage = critical, digitalSignature
extendedKeyUsage = codeSigning" > code_signing_ext.cnf

openssl x509 -req -in ecdsa.csr -CA ca.crt -CAkey ca.key -CAcreateserial -outform der -out ecdsa.crt -days 365 -sha256 -extfile code_signing_ext.cnf -extensions v3_code_sign 
```
#### Add CA to trusted root
```
cp ca.crt ~/.swiftpm/security/trusted-root-certs/ca.cer
```
##### trusted store configuration [~/.swiftpm/configuration/registries.json]
(https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/PackageRegistryUsage.md#security-configuration)
```
{
    "security": {
    "default": {
      "signing": {
        "onUnsigned": "error",
        "onUntrustedCertificate": "error",
        "trustedRootCertificatesPath": "/Users/[user]/.swiftpm/security/trusted-root-certs/",
        "includeDefaultTrustedRootCertificates": true,
        "validationChecks": {
          "certificateExpiration": "disabled",
          "certificateRevocation": "disabled"
        }
      }
    }
  },
  ...
}
```
#### publish with signing
```
swift package-registry publish [scope].[Package] [version] --metadata-path package-metadata.json --vv --private-key-path ecdsa_pkcs8.key --cert-chain-paths ecdsa.crt
```
### 📋 Todos ❎
- ✔️ Publishing
    - ❌ Package Validity checking (checksum, manifest, etc)
    - ❌ asynchronous
    - ❌ base64 format
- ❌ Delete packages
- ✔️ Storage
  - ❌ DB support (e.g. mysql, postgres)
  - ❌ online storage (e.g S3, cloud drive)
- ❌ User Management / Access Control
  - ❌ Basic Auth
  - ❌ Oauth token