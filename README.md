![GitHub release (latest by date)](https://img.shields.io/github/v/release/wgr1984/openspmregistry)
[![Docker Hub](https://img.shields.io/docker/pulls/wgr1984/openspmregistry)](https://hub.docker.com/r/wgr1984/openspmregistry)

<table cellspacing="0" cellpadding="0" style="border: none">
  <tr>
    <td>
      <img src="static/favicon.svg" style="padding-top: 30px; padding-bottom: 20px" height="200">
    </td>
    <td>
      <pre>
  ___                   ____  ____  __  __ ____            _     _              
 / _ \ _ __   ___ _ __ / ___||  _ \|  \/  |  _ \ ___  __ _(_)___| |_ _ __ _   _ 
| | | | '_ \ / _ \ '_ \\___ \| |_) | |\/| | |_) / _ \/ _` | / __| __| '__| | | |
| |_| | |_) |  __/ | | |___) |  __/| |  | |  _ <  __/ (_| | \__ \ |_| |  | |_| |
 \___/| .__/ \___|_| |_|____/|_|   |_|  |_|_| \_\___|\__, |_|___/\__|_|   \__, |
      |_|                                            |___/                |___/ 
      </pre>
    </td>
  </tr>
</table>
  
Simple (using as much go std. lib and as less external dependencies as possible) implementation of Swift Package Manager Registry according to
https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/Registry.md


# How To Use
There is a complete documentation available at https://wgr1984.github.io/docs/openspmregistry

Quick links:
- [Getting Started](https://wgr1984.github.io/docs/openspmregistry/gettingstarted)
- [Documentation](https://wgr1984.github.io/docs/openspmregistry/documention)

# Features
Browsing, Publishing (including signing) and retrieving of swift packages
[More](https://wgr1984.github.io/docs/openspmregistry/#features)

## Use source
```
git clone https://github.com/wgr1984/openspmregistry.git
```
copy `config.yml` to `config.yml.local` and adjust to your needs
```yaml
# config.local.yml:
server:
  hostname: 127.0.0.1
  port: 8080
  certs:
    cert: server.crt
    key: server.key
  repo:
    type: file
    path: ./files/
  publish:
    maxSize: 204800
  auth:
    enabled: false
```
hit go run:
```
 go run main.go -tls=true -v
```

# 📋 Todos ❎
[Roadmap](https://wgr1984.github.io/docs/openspmregistry/#roadmap)

# License
[BSD-3-Clause Licence](LICENSE)

# Disclaimer
This project is in an early stage and used in production at your own risk.
This project is a common use project and not affiliated with Apple Inc. or the Swift project.