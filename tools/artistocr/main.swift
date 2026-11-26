// artistocr reads the artist line of KeyForge card images with the macOS
// Vision text recognizer. It needs no install: Vision is part of macOS.
//
// Usage: artistocr [known-artists-file] < paths
//
// It reads image paths from stdin, one per line. For each image it crops
// the artist strip at the bottom left, makes 12 variants (3 scales, color
// or inverted grey, with or without the known artists as custom words), and
// writes one JSON line with all the readings. The kfdata artists-scan
// command matches the readings to the known artists.
import CoreImage
import Foundation
import Vision

struct Reading: Codable {
    let variant: String
    let text: String
    let conf: Float
}

struct Result: Codable {
    let path: String
    let readings: [Reading]
    let error: String?
}

let ctx = CIContext()

// The custom words are the known names and each word of them.
let words: [String] = {
    guard CommandLine.arguments.count > 1,
          let s = try? String(contentsOfFile: CommandLine.arguments[1], encoding: .utf8) else { return [] }
    let names = s.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    return names + names.flatMap { $0.split(separator: " ").map(String.init) }
}()

// strip crops the artist name in a 300x420 card (x 30..104, y 382..400 from
// the top) and scales it up. Other image sizes scale the box.
func strip(_ img: CIImage, scale: CGFloat) -> CIImage {
    let sx = img.extent.width / 300, sy = img.extent.height / 420
    // CoreImage has the origin at the bottom left.
    let r = CGRect(x: 30 * sx, y: 20 * sy, width: 74 * sx, height: 18 * sy)
    let c = img.cropped(to: r).transformed(by: CGAffineTransform(translationX: -r.minX, y: -r.minY))
    let f = CIFilter(name: "CILanczosScaleTransform")!
    f.setValue(c, forKey: kCIInputImageKey)
    f.setValue(scale, forKey: kCIInputScaleKey)
    f.setValue(1.0, forKey: kCIInputAspectRatioKey)
    let out = f.outputImage!
    return out.composited(over: CIImage(color: .white).cropped(to: out.extent))
}

// invertGray makes the white text on a colored bar dark text on a light page.
func invertGray(_ img: CIImage) -> CIImage {
    img.applyingFilter("CIColorControls", parameters: [kCIInputSaturationKey: 0, kCIInputContrastKey: 1.6])
        .applyingFilter("CIColorInvert")
}

// pad adds a white margin: Vision reads a small single line better with one.
func pad(_ img: CIImage) -> CIImage {
    img.composited(over: CIImage(color: .white).cropped(to: img.extent.insetBy(dx: -40, dy: -40)))
}

func recognize(_ img: CIImage, custom: Bool) -> Reading? {
    guard let cg = ctx.createCGImage(img, from: img.extent) else { return nil }
    let req = VNRecognizeTextRequest()
    req.recognitionLevel = .accurate
    req.usesLanguageCorrection = custom
    if custom { req.customWords = words }
    try? VNImageRequestHandler(cgImage: cg).perform([req])
    let tops = (req.results ?? []).compactMap { $0.topCandidates(1).first }
        .filter { $0.string.rangeOfCharacter(from: .letters) != nil }
    guard let conf = tops.map(\.confidence).min() else { return nil }
    return Reading(variant: "", text: tops.map(\.string).joined(separator: " "), conf: conf)
}

func ocr(_ path: String) -> Result {
    guard let src = CIImage(contentsOf: URL(fileURLWithPath: path)) else {
        return Result(path: path, readings: [], error: "cannot read the image")
    }
    var readings: [Reading] = []
    for scale in [3, 4, 6] {
        let s = strip(src, scale: CGFloat(scale))
        for (name, v) in [("color", pad(s)), ("inv", pad(invertGray(s)))] {
            for custom in words.isEmpty ? [false] : [false, true] {
                if let r = recognize(v, custom: custom) {
                    let id = "\(name)\(scale)\(custom ? "w" : "")"
                    readings.append(Reading(variant: id, text: r.text, conf: r.conf))
                }
            }
        }
    }
    return Result(path: path, readings: readings, error: nil)
}

let enc = JSONEncoder()
while let line = readLine() {
    let p = line.trimmingCharacters(in: .whitespaces)
    if p.isEmpty { continue }
    FileHandle.standardOutput.write(try! enc.encode(ocr(p)) + "\n".data(using: .utf8)!)
}
