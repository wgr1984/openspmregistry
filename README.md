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
```
TBD
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