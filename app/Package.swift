// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Twin",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(name: "Twin", path: "Sources/Twin")
    ]
)
