// swift-tools-version:5.9

import PackageDescription

let package = Package(
    name: "Consumer",
    dependencies: [
        .package(id: "example.SamplePackage", from: "1.0.0"),
        .package(id: "example.UtilsPackage", from: "1.0.0"),
    ],
    targets: [
        .executableTarget(
            name: "Consumer",
            dependencies: [
                .product(name: "SamplePackage", package: "example.SamplePackage"),
                .product(name: "UtilsPackage", package: "example.UtilsPackage"),
            ]
        ),
    ]
)
