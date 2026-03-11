// swift-tools-version:6.0

import PackageDescription

let package = Package(
    name: "Consumer",
    platforms: [.macOS(.v12)],
    dependencies: [
        .package(id: "example.SamplePackage", from: "1.0.0"),
        .package(id: "example.UtilsPackage", from: "1.0.0"),
        .package(id: "example.SwiftSignedPkg", from: "1.0.0"),
    ],
    targets: [
        .executableTarget(
            name: "Consumer",
            dependencies: [
                .product(name: "SamplePackage", package: "example.SamplePackage"),
                .product(name: "UtilsPackage", package: "example.UtilsPackage"),
                .product(name: "SwiftSignedPkg", package: "example.SwiftSignedPkg"),
            ]
        ),
    ]
)
