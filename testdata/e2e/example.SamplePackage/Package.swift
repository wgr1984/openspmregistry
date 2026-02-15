// swift-tools-version:5.9

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
