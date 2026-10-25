#!/usr/bin/env swift
import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

// Convert the owner's artwork without cropping, redrawing, or losing transparency.
let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
let source = root.appendingPathComponent("assets/branding/megafloyd-arcade.png")
let fm = FileManager.default
let stamp = ISO8601DateFormatter().string(from: Date()).replacingOccurrences(of: ":", with: "")
let backup = root.appendingPathComponent(".tmp/icon-backups-\(stamp)")

func fail(_ message: String) -> Never {
    fputs("\(message)\n", stderr)
    exit(1)
}

guard let imageSource = CGImageSourceCreateWithURL(source as CFURL, nil),
      let artwork = CGImageSourceCreateImageAtIndex(imageSource, 0, nil) else {
    fail("Cannot read canonical artwork: \(source.path)")
}

func png(_ size: Int) -> Data {
    guard let colorSpace = CGColorSpace(name: CGColorSpace.sRGB),
          let context = CGContext(data: nil, width: size, height: size, bitsPerComponent: 8,
                                  bytesPerRow: size * 4, space: colorSpace,
                                  bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else {
        fail("Cannot create icon context")
    }
    context.clear(CGRect(x: 0, y: 0, width: size, height: size))
    context.interpolationQuality = .high
    let scale = Double(size) * 0.96 / Double(max(artwork.width, artwork.height))
    let width = Double(artwork.width) * scale
    let height = Double(artwork.height) * scale
    context.draw(artwork, in: CGRect(x: (Double(size) - width) / 2,
                                    y: (Double(size) - height) / 2, width: width, height: height))
    guard let image = context.makeImage() else { fail("Cannot render icon") }
    let data = NSMutableData()
    guard let destination = CGImageDestinationCreateWithData(data, UTType.png.identifier as CFString, 1, nil) else {
        fail("Cannot encode icon")
    }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else { fail("Cannot finish icon PNG") }
    return data as Data
}

func write(_ data: Data, _ relative: String) throws {
    let target = root.appendingPathComponent(relative)
    if fm.fileExists(atPath: target.path) {
        if try Data(contentsOf: target) == data { return }
        let saved = backup.appendingPathComponent(relative)
        try fm.createDirectory(at: saved.deletingLastPathComponent(), withIntermediateDirectories: true)
        try fm.copyItem(at: target, to: saved)
    }
    try fm.createDirectory(at: target.deletingLastPathComponent(), withIntermediateDirectories: true)
    try data.write(to: target, options: .atomic)
    print("Updated \(relative)")
}

func le16(_ value: Int) -> Data { Data([UInt8(value & 255), UInt8((value >> 8) & 255)]) }
func le32(_ value: Int) -> Data { le16(value & 65535) + le16((value >> 16) & 65535) }

do {
    let sizes = [16, 32, 48, 64, 128, 180, 192, 256, 512, 1024]
    let images = Dictionary(uniqueKeysWithValues: sizes.map { ($0, png($0)) })
    let iconset = root.appendingPathComponent(".tmp/megafloyd-arcade-\(UUID().uuidString).iconset")
    try fm.createDirectory(at: iconset, withIntermediateDirectories: true)
    defer { try? fm.removeItem(at: iconset) }
    for size in [16, 32, 128, 256, 512] {
        try images[size]!.write(to: iconset.appendingPathComponent("icon_\(size)x\(size).png"))
        try images[size * 2]!.write(to: iconset.appendingPathComponent("icon_\(size)x\(size)@2x.png"))
    }
    let icns = iconset.appendingPathComponent("icon.icns")
    let process = Process()
    process.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
    process.arguments = ["-c", "icns", iconset.path, "-o", icns.path]
    try process.run()
    process.waitUntilExit()
    guard process.terminationStatus == 0 else { fail("iconutil failed") }
    try write(Data(contentsOf: icns), "desktop/desktop/assets/icon.icns")
    try write(images[1024]!, "desktop/desktop/assets/icon-1024.png")
    let web = "floyd-code/apps/floyd-code/dist-web/"
    for size in [32, 180, 192, 512] {
        try write(images[size]!, "\(web)megafloyd-icon-\(size).png")
    }
    let icoSizes = [16, 32, 48, 64, 128, 256]
    var ico = le16(0) + le16(1) + le16(icoSizes.count)
    var offset = 6 + icoSizes.count * 16
    for size in icoSizes {
        let data = images[size]!
        ico += Data([UInt8(size % 256), UInt8(size % 256), 0, 0])
        ico += le16(1) + le16(32) + le32(data.count) + le32(offset)
        offset += data.count
    }
    for size in icoSizes { ico += images[size]! }
    try write(ico, "\(web)favicon.ico")
    try write(ico, "floyd-code/docs/public/favicon.ico")
    for filename in ["floyd-icon.png", "floyd-icon-solo.png"] {
        try write(images[512]!, "floyd-go/internal/ui/notification/\(filename)")
    }
    try write(images[512]!, "floyd-code/apps/vscode/resources/floyd-icon-storefront.png")
    let svg = "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"512\" height=\"512\" viewBox=\"0 0 512 512\"><title>MegaFloyd</title><image width=\"512\" height=\"512\" href=\"data:image/png;base64,\(images[512]!.base64EncodedString())\"/></svg>\n"
    try write(Data(svg.utf8), "floyd-code/apps/vscode/resources/floyd-icon.svg")
    print("MegaFloyd icons generated from \(source.path); artwork preserved at its original aspect ratio.")
} catch { fail(String(describing: error)) }
