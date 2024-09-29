```
  ___                   ____  ____  __  __ ____            _     _              
 / _ \ _ __   ___ _ __ / ___||  _ \|  \/  |  _ \ ___  __ _(_)___| |_ _ __ _   _ 
| | | | '_ \ / _ \ '_ \\___ \| |_) | |\/| | |_) / _ \/ _` | / __| __| '__| | | |
| |_| | |_) |  __/ | | |___) |  __/| |  | |  _ <  __/ (_| | \__ \ |_| |  | |_| |
 \___/| .__/ \___|_| |_|____/|_|   |_|  |_|_| \_\___|\__, |_|___/\__|_|   \__, |
      |_|                                            |___/                |___/ 
```

Simple implementation of Swift Package Manager Registry according to
https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/Registry.md

# Current Features

# How To Use
## Run server
## Usage in SPM
### Create New Project
https://www.swift.org/documentation/package-manager/
e.g.
```
mkdir spm_test
cd spm_test
swift package init --type=executable 
```
ensure spm registry is known and setup
e.g. `localhost` (be ware `swift package-registry` as for now accepts tls/ssl connections only)
```
swift package-registry set https://127.0.0.1:1234
```
⚠️ on local setup we need to make sure ssl cert is set too trusted on system level