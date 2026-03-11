// swift-tools-version:6.0

import PackageDescription

let package = Package(
    name: "SamplePackage",
    products: [
        .library(name: "SamplePackage", targets: ["SamplePackage"]),
    ],
    targets: [
        .target(name: "SamplePackage"),
    ]
)
