// swift-tools-version:5.8

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
