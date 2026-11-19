// make-icon <out.iconset>: draws inkstudio's icon at every size an .icns
// needs. `make icon` turns the iconset into packaging/inkstudio.icns.
//
// The icon is an ink-blue tile, on Apple's icon grid, holding a slide: a
// white 16:9 card with a title bar and lines of text, and a smaller card
// behind it, for a deck.
import AppKit

let out = CommandLine.arguments[1]
try? FileManager.default.createDirectory(atPath: out, withIntermediateDirectories: true)

func draw(_ px: Int) -> Data {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                               colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    let s = CGFloat(px) / 1024
    let ctx = NSGraphicsContext.current!.cgContext
    ctx.scaleBy(x: s, y: s)

    // The tile: 824 pt on a 1024 canvas, corner radius 185, with a soft shadow.
    let tile = NSRect(x: 100, y: 100, width: 824, height: 824)
    let tilePath = NSBezierPath(roundedRect: tile, xRadius: 185, yRadius: 185)
    NSGraphicsContext.saveGraphicsState()
    let shadow = NSShadow()
    shadow.shadowColor = NSColor(white: 0, alpha: 0.35)
    shadow.shadowBlurRadius = 24
    shadow.shadowOffset = NSSize(width: 0, height: -10)
    shadow.set()
    NSColor(red: 0.07, green: 0.13, blue: 0.36, alpha: 1).setFill()
    tilePath.fill()
    NSGraphicsContext.restoreGraphicsState()
    NSGradient(colors: [NSColor(red: 0.20, green: 0.45, blue: 1.0, alpha: 1),
                        NSColor(red: 0.06, green: 0.16, blue: 0.48, alpha: 1)])!
        .draw(in: tilePath, angle: -70)

    // The card behind, for a deck.
    let back = NSRect(x: 262, y: 452, width: 560, height: 315)
    NSColor(white: 1, alpha: 0.35).setFill()
    NSBezierPath(roundedRect: back, xRadius: 28, yRadius: 28).fill()

    // The slide.
    let card = NSRect(x: 202, y: 312, width: 620, height: 349)
    NSGraphicsContext.saveGraphicsState()
    let cardShadow = NSShadow()
    cardShadow.shadowColor = NSColor(white: 0, alpha: 0.3)
    cardShadow.shadowBlurRadius = 30
    cardShadow.shadowOffset = NSSize(width: 0, height: -12)
    cardShadow.set()
    NSColor.white.setFill()
    NSBezierPath(roundedRect: card, xRadius: 30, yRadius: 30).fill()
    NSGraphicsContext.restoreGraphicsState()

    // A title and lines of text on the slide.
    let ink = NSColor(red: 0.06, green: 0.16, blue: 0.48, alpha: 1)
    ink.setFill()
    NSBezierPath(roundedRect: NSRect(x: 252, y: 566, width: 300, height: 44), xRadius: 22, yRadius: 22).fill()
    NSColor(red: 0.20, green: 0.45, blue: 1.0, alpha: 1).setFill()
    NSBezierPath(roundedRect: NSRect(x: 252, y: 520, width: 120, height: 14), xRadius: 7, yRadius: 7).fill()
    NSColor(white: 0.78, alpha: 1).setFill()
    for (i, w) in [440.0, 380.0, 410.0].enumerated() {
        NSBezierPath(roundedRect: NSRect(x: 252, y: 460 - Double(i) * 42, width: w, height: 18), xRadius: 9, yRadius: 9).fill()
    }

    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

for (name, px) in [("16x16", 16), ("16x16@2x", 32), ("32x32", 32), ("32x32@2x", 64),
                   ("128x128", 128), ("128x128@2x", 256), ("256x256", 256),
                   ("256x256@2x", 512), ("512x512", 512), ("512x512@2x", 1024)] {
    try! draw(px).write(to: URL(fileURLWithPath: "\(out)/icon_\(name).png"))
}
print("wrote \(out)")
