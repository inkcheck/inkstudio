// webkit-snap <url> <out.png> <width> <height> [delay] [script]: loads url in
// an offscreen WKWebView, the engine the macOS app runs in, optionally runs
// script a second after it loads, and saves a snapshot after delay seconds.
// Use it with `make web` to see the UI as the app draws it.
import AppKit
import WebKit

let args = CommandLine.arguments
let url = URL(string: args[1])!, out = args[2]
let w = Double(args[3])!, h = Double(args[4])!
let delay = args.count > 5 ? Double(args[5])! : 1.5
let script = args.count > 6 ? args[6] : ""

class Delegate: NSObject, WKNavigationDelegate {
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        if !script.isEmpty {
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) {
                webView.evaluateJavaScript(script) { _, err in if let err = err { print("js error: \(err)") } }
            }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) {
            let cfg = WKSnapshotConfiguration()
            webView.takeSnapshot(with: cfg) { image, error in
                guard let image = image, let tiff = image.tiffRepresentation,
                      let rep = NSBitmapImageRep(data: tiff),
                      let png = rep.representation(using: .png, properties: [:]) else {
                    print("snapshot failed: \(String(describing: error))"); exit(1)
                }
                try! png.write(to: URL(fileURLWithPath: out))
                print("wrote \(out)"); exit(0)
            }
        }
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { print("fail \(error)"); exit(1) }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { print("fail \(error)"); exit(1) }
}

let app = NSApplication.shared
app.setActivationPolicy(.prohibited)
let window = NSWindow(contentRect: NSRect(x: -10000, y: -10000, width: w, height: h), styleMask: [.borderless], backing: .buffered, defer: false)
let web = WKWebView(frame: NSRect(x: 0, y: 0, width: w, height: h))
let delegate = Delegate()
web.navigationDelegate = delegate
window.contentView = web
web.load(URLRequest(url: url))
DispatchQueue.main.asyncAfter(deadline: .now() + 20) { print("timeout"); exit(1) }
app.run()
