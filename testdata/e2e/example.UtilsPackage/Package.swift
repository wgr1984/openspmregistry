// swift-tools-version:6.0

import PackageDescription

let package = Package(
    name: "UtilsPackage",
    products: [
        .library(name: "UtilsPackage", targets: ["UtilsPackage"]),
    ],
    targets: [
        .target(name: "UtilsPackage"),
    ]
)
