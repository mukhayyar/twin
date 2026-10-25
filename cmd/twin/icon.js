// icon.js paints a clone's icon: shifts its dominant hue to the label color
// and draws a label badge, writing a .iconset for iconutil.
//
//   osascript -l JavaScript icon.js <app> <label> <#RRGGBB> <tint 0|1> <badge 0|1> <out.iconset> [preview.png]
ObjC.import('AppKit');
ObjC.import('CoreImage');

const S = 1024;

function run(argv) {
  const [appPath, label, hex, tint, badge, outDir, preview] = argv;
  const color = colorFromHex(hex);

  const original = newRep(S);
  draw(original, () => {
    $.NSWorkspace.sharedWorkspace.iconForFile(appPath).drawInRectFromRectOperationFraction(
      $.NSMakeRect(0, 0, S, S), $.NSZeroRect, $.NSCompositingOperationSourceOver, 1);
  });

  let image = $.CIImage.alloc.initWithBitmapImageRep(original);
  if (tint === '1') image = shiftHue(image, color);

  const final = newRep(S);
  draw(final, () => {
    $.NSCIImageRep.imageRepWithCIImage(image).drawInRect($.NSMakeRect(0, 0, S, S));
    if (badge === '1') drawBadge(label.replace(/-/g, ' '), color);
  });

  const sizes = [[16, '16x16'], [32, '16x16@2x'], [32, '32x32'], [64, '32x32@2x'],
    [128, '128x128'], [256, '128x128@2x'], [256, '256x256'], [512, '256x256@2x'],
    [512, '512x512'], [1024, '512x512@2x']];
  for (const [px, name] of sizes) writePNG(scaled(final, px), `${outDir}/icon_${name}.png`);
  if (preview) writePNG(final, preview);
}

// shiftHue rotates hues so the icon's average hue lands on the target color.
// Near-grayscale icons are left alone; the badge carries the identity.
// The rotation happens in HSV (keeping saturation and value), unlike
// CIHueAdjust, which keeps luma and turns e.g. blue into muddy brown.
function shiftHue(image, target) {
  const avg = $.CIFilter.filterWithName('CIAreaAverage');
  avg.setValueForKey(image, 'inputImage');
  avg.setValueForKey($.CIVector.vectorWithXYZW(0, 0, S, S), 'inputExtent');
  const px = newRep(1);
  draw(px, () => $.NSCIImageRep.imageRepWithCIImage(avg.outputImage).drawInRect($.NSMakeRect(0, 0, 1, 1)));
  const c = px.colorAtXY(0, 0).colorUsingColorSpace($.NSColorSpace.deviceRGBColorSpace);
  if (c.saturationComponent < 0.15) return image;

  const n = 32;
  const cube = new Float32Array(n * n * n * 4);
  const delta = target.hueComponent - c.hueComponent;
  // Pull saturation toward the target's so pastel results don't look washed out.
  const gain = Math.min(1.8, Math.max(0.6, target.saturationComponent / c.saturationComponent));
  let i = 0;
  for (let b = 0; b < n; b++) for (let g = 0; g < n; g++) for (let r = 0; r < n; r++) {
    const [h, s, v] = rgbToHsv(r / (n - 1), g / (n - 1), b / (n - 1));
    const [R, G, B] = hsvToRgb((h + delta + 1) % 1, Math.min(1, s * gain), v);
    cube[i++] = R; cube[i++] = G; cube[i++] = B; cube[i++] = 1;
  }
  // The cube is built from sRGB values but Core Image works in linear light,
  // so convert around it (JXA can't hand a CGColorSpace to the filter).
  const filter = $.CIFilter.filterWithName('CIColorCube');
  filter.setValueForKey(applyFilter('CILinearToSRGBToneCurve', image), 'inputImage');
  filter.setValueForKey(n, 'inputCubeDimension');
  filter.setValueForKey($.NSData.alloc.initWithBase64EncodedStringOptions(base64(new Uint8Array(cube.buffer)), 0), 'inputCubeData');
  return applyFilter('CISRGBToneCurveToLinear', filter.outputImage);
}

function applyFilter(name, image) {
  const f = $.CIFilter.filterWithName(name);
  f.setValueForKey(image, 'inputImage');
  return f.outputImage;
}

function rgbToHsv(r, g, b) {
  const max = Math.max(r, g, b), d = max - Math.min(r, g, b);
  let h = 0;
  if (d) h = max === r ? ((g - b) / d + 6) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
  return [h / 6, max ? d / max : 0, max];
}

function hsvToRgb(h, s, v) {
  const f = (k) => { const x = (k + h * 6) % 6; return v - v * s * Math.max(0, Math.min(x, 4 - x, 1)); };
  return [f(5), f(3), f(1)];
}

// base64 exists because JXA has no btoa and NSData can't wrap a typed array.
function base64(bytes) {
  const abc = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const n = (bytes[i] << 16) | ((bytes[i + 1] || 0) << 8) | (bytes[i + 2] || 0);
    out += abc[(n >> 18) & 63] + abc[(n >> 12) & 63] +
      (i + 1 < bytes.length ? abc[(n >> 6) & 63] : '=') + (i + 2 < bytes.length ? abc[n & 63] : '=');
  }
  return out;
}

function drawBadge(text, color) {
  const h = 0.20 * S, y = S - 0.955 * S, pad = 0.11 * S, maxW = 0.94 * S;

  // JXA can't construct NSAttributedString, so use NSString's drawing methods.
  let size = 0.125 * S, font, attrs, str;
  const layout = (t) => {
    font = roundedFont(size);
    attrs = $.NSMutableDictionary.dictionary;
    attrs.setObjectForKey(font, 'NSFont');
    attrs.setObjectForKey($.NSColor.whiteColor, 'NSColor');
    str = $(t);
    return str.sizeWithAttributes(attrs).width + pad;
  };
  while (layout(text) > maxW && size > 0.075 * S) size -= 0.005 * S;
  for (let t = text; layout(t) > maxW && t.length > 1;) t = t.slice(0, -2) + '…';

  const textW = str.sizeWithAttributes(attrs).width;
  const w = textW + pad;
  const pill = $.NSBezierPath.bezierPathWithRoundedRectXRadiusYRadius($.NSMakeRect((S - w) / 2, y, w, h), h / 2, h / 2);

  $.NSGraphicsContext.saveGraphicsState;
  const shadow = $.NSShadow.alloc.init;
  shadow.setShadowColor($.NSColor.colorWithWhiteAlpha(0, 0.35));
  shadow.setShadowBlurRadius(0.02 * S);
  shadow.setShadowOffset($.NSMakeSize(0, -0.008 * S));
  shadow.set;
  color.setFill;
  pill.fill;
  $.NSGraphicsContext.restoreGraphicsState;

  $.NSColor.whiteColor.setStroke;
  pill.setLineWidth(0.018 * S);
  pill.stroke;

  // Center the cap height on the pill; drawAtPoint takes the descender corner.
  const baseline = y + h / 2 - font.capHeight / 2;
  str.drawAtPointWithAttributes($.NSMakePoint((S - textW) / 2, baseline + font.descender), attrs);
}

function roundedFont(size) {
  const font = $.NSFont.systemFontOfSizeWeight(size, 0.56); // NSFontWeightHeavy
  const desc = font.fontDescriptor.fontDescriptorWithDesign('NSCTFontUIFontDesignRounded');
  if (desc.isNil()) return font;
  const rounded = $.NSFont.fontWithDescriptorSize(desc, size);
  return rounded.isNil() ? font : rounded;
}

function colorFromHex(hex) {
  const n = parseInt(hex.replace('#', ''), 16);
  return $.NSColor.colorWithDeviceRedGreenBlueAlpha(((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255, 1);
}

function newRep(px) {
  return $.NSBitmapImageRep.alloc.initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(
    null, px, px, 8, 4, true, false, 'NSDeviceRGBColorSpace', 0, 0);
}

function draw(rep, fn) {
  $.NSGraphicsContext.saveGraphicsState;
  const ctx = $.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep);
  ctx.setImageInterpolation($.NSImageInterpolationHigh);
  $.NSGraphicsContext.setCurrentContext(ctx);
  fn();
  ctx.flushGraphics;
  $.NSGraphicsContext.restoreGraphicsState;
}

function scaled(rep, px) {
  if (px === S) return rep;
  const out = newRep(px);
  draw(out, () => rep.drawInRect($.NSMakeRect(0, 0, px, px)));
  return out;
}

function writePNG(rep, path) {
  rep.representationUsingTypeProperties(4 /* NSBitmapImageFileTypePNG */, $.NSDictionary.dictionary)
    .writeToFileAtomically(path, true);
}
