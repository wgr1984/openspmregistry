// swift-tools-version:5.9

import PackageDescription

let package = Package(
    name: "Consumer",
    dependencies: [
        .package(id: "example.SamplePackage", from: "1.0.0"),
    ],
    targets: [
        .executableTarget(
            name: "Consumer",
            dependencies: ["SamplePackage"]
        ),
    ]
)
