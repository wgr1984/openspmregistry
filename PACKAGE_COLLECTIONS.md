# Package Collections

This registry supports [SE-0291 Package Collections](https://github.com/swiftlang/swift-evolution/blob/main/proposals/0291-package-collections.md), allowing clients to discover packages through curated collections.

## Overview

Package Collections provide a way to discover Swift packages through curated lists. This registry automatically generates collections from published packages, making them accessible to Swift Package Manager and Xcode.

## Endpoints

### Global Collection

Get a collection of all packages in the registry:

```
GET /collection
Accept: application/json
```

**Response:**
```json
{
  "name": "localhost: All Packages",
  "overview": "All packages in registry",
  "packages": [...],
  "formatVersion": "1.0",
  "generatedAt": "2024-01-06T12:00:00Z",
  "generatedBy": {
    "name": "OpenSPMRegistry"
  }
}
```

### Scope-Specific Collection

Get a collection of packages in a specific scope:

```
GET /collection/{scope}
Accept: application/json
```

**Example:**
```bash
curl -H "Accept: application/json" https://registry.example.com/collection/ext
```

## Configuration

Package Collections can be enabled/disabled in `config.yml`:

```yaml
server:
  packageCollections:
    enabled: true              # Enable/disable collections endpoints
    requirePackageJson: false  # If true, publish fails without Package.json
    allowAuthQueryParam: false # If true, allow ?auth= on collection URLs only (e.g. swift package-collection add)
```

### Configuration Options

- **enabled** (boolean): Enable or disable package collections endpoints
  - `true`: Collections endpoints are available
  - `false`: Collections endpoints return 404

- **requirePackageJson** (boolean): Make Package.json mandatory for publishing
  - `false` (default): Packages can be published without Package.json (they won't appear in collections)
  - `true`: Publishing fails if Package.json is not included in the archive

- **publicRead** (boolean): Allow unauthenticated read access to collection endpoints
  - `false` (default): Collections require auth when server auth is enabled
  - `true`: `GET /collection` and `GET /collection/{scope}` are public

- **allowAuthQueryParam** (boolean): Allow passing credentials via the `auth` query parameter on **collection paths only**
  - `false` (default): Query param is ignored; avoids credential leakage via logs, referrers, and proxies
  - `true`: Enables `?auth=<base64(full Authorization value)>` for clients that cannot send headers (e.g. `swift package-collection add`). The decoded value must start with `Basic ` or `Bearer `.

When auth is required and `allowAuthQueryParam` is true, use `?auth=<base64(full Authorization value)>` on collection URLs — e.g. base64 of `Basic YWRtaW46YWRtaW4xMjM=` or `Bearer token`.

## For Package Publishers

### Including Package.json in Your Package

To have your package appear in registry collections, include a `Package.json` file in your source archive:

#### 1. Generate Package.json

From your package directory, run:

```bash
swift package dump-package > Package.json
```

This generates a JSON file with your package's manifest information.

#### 2. Include in Source Archive

When creating your source archive, ensure `Package.json` is at the root level alongside `Package.swift`:

```
YourPackage/
├── Package.swift
├── Package.json          ← Include this
├── Sources/
└── Tests/
```

#### 3. Publish Normally

Publish your package as usual:

```bash
swift package-registry publish scope package-name version archive.zip
```

The registry will automatically:
- Extract `Package.json` from your archive
- Use it to generate collection metadata
- Include your package in collections

### What Happens Without Package.json?

- If `requirePackageJson: false` (default):
  - Your package publishes successfully
  - It will NOT appear in package collections
  - All other registry features work normally

- If `requirePackageJson: true`:
  - Publishing fails with HTTP 422 if Package.json is missing
  - You must include Package.json to publish

### Package.json Contents

The `Package.json` file contains:
- Package name
- Products and targets
- Platform requirements
- Swift tools version
- Dependencies

Example structure:
```json
{
  "name": "YourPackage",
  "platforms": [
    {
      "platformName": "macos",
      "version": "10.13"
    },
    {
      "platformName": "ios",
      "version": "12.0"
    }
  ],
  "products": [
    {
      "name": "YourPackage",
      "targets": ["YourPackage"],
      "type": {
        "library": ["automatic"]
      }
    }
  ],
  "targets": [
    {
      "name": "YourPackage",
      "type": "regular"
    }
  ],
  "toolsVersion": {
    "_version": "5.10.0"
  }
}
```

## Using Collections in Xcode

### Adding a Collection

1. In Xcode, go to **File > Add Package Dependencies**
2. Click the **+** button at the bottom left
3. Select **Add Package Collection**
4. Enter the collection URL: `https://your-registry.com/collection`

### Using Registry-Native Package IDs

This registry uses `scope.name` format as package URLs in collections (e.g., `ext.Alamofire`). This allows Xcode to recognize packages as registry-native and resolve them through the registry rather than Git.

## Collection Format

Collections follow the [SE-0291 v1.0 format](https://github.com/apple/swift-package-collection-generator/blob/main/PackageCollectionFormats/v1.md).

### Key Features

- **Package Identification**: Uses `scope.name` format (e.g., `ext.Alamofire`)
- **Version Information**: Includes all published versions with Package.json
- **Manifest Details**: Products, targets, and platform requirements
- **Metadata**: Author, license, description from package metadata
- **Dynamic Generation**: Collections are generated on-demand from latest data

### Metadata Sources

Collection metadata is populated from:
1. **Package.json**: Manifest structure, products, targets, platforms
2. **Metadata.json**: Author, description, license, README URL (from publish)
3. **Package.swift**: Swift tools version
4. **Registry**: Publish dates

## Benefits

### For Package Consumers

- **Easy Discovery**: Find packages through curated collections
- **Rich Metadata**: See platform support, products, and requirements before adding
- **Xcode Integration**: Browse collections directly in Xcode
- **Up-to-Date**: Collections reflect current published packages

### For Package Publishers

- **Increased Visibility**: Packages appear in searchable collections
- **Better Presentation**: Rich metadata helps users understand your package
- **No Extra Work**: Just include Package.json (one command)
- **Automatic Updates**: Collections update when you publish new versions

## Technical Details

### Collection Generation

- Collections are generated dynamically on each request
- Only versions with Package.json are included
- Packages with no valid versions are excluded
- Failed Package.json parsing logs a warning but doesn't fail the collection

### Authentication

Collection endpoints respect the registry's authentication settings:
- If authentication is enabled, collections require authentication
- If authentication is disabled, collections are publicly accessible

### Caching

Clients should cache collection responses:
- Collections can be large for registries with many packages
- Generation can take time for large collections
- Set appropriate cache headers in your HTTP client

## Troubleshooting

### Package Not Appearing in Collection

**Cause**: Package.json is missing or invalid

**Solution**:
1. Verify Package.json is in your source archive
2. Regenerate: `swift package dump-package > Package.json`
3. Republish the package version

### Collection Returns 404

**Cause**: Collections are disabled

**Solution**: Enable in config:
```yaml
packageCollections:
  enabled: true
```

### Publish Fails with "Package.json required"

**Cause**: `requirePackageJson: true` but Package.json is missing

**Solution**: Include Package.json in your archive or set `requirePackageJson: false`

## Examples

### Complete Publishing Workflow

```bash
# 1. Navigate to your package
cd MyPackage

# 2. Generate Package.json
swift package dump-package > Package.json

# 3. Create source archive (Package.json will be included)
swift package archive-source

# 4. Publish
swift package-registry publish my-scope MyPackage 1.0.0 MyPackage-1.0.0.zip

# Your package now appears in collections!
```

### Fetching a Collection

```bash
# Global collection
curl -H "Accept: application/json" \
     https://registry.example.com/collection

# Scope-specific collection
curl -H "Accept: application/json" \
     https://registry.example.com/collection/my-scope
```

## See Also

- [SE-0291 Package Collections Proposal](https://github.com/swiftlang/swift-evolution/blob/main/proposals/0291-package-collections.md)
- [Package Collection Format v1.0](https://github.com/apple/swift-package-collection-generator/blob/main/PackageCollectionFormats/v1.md)
- [Swift Package Manager Documentation](https://github.com/apple/swift-package-manager/tree/main/Documentation)

