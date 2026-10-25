package main

// Windows twins are Start Menu shortcuts. Shortcuts are COM objects, so twin
// drives them through PowerShell; building those scripts and the launcher
// command line is string work, kept here without a build tag so it's tested
// on every platform.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"strings"
)

// psQuote makes a PowerShell single-quoted string literal.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// winLauncherArgs is the cmd.exe command line a twin's shortcut runs: it
// points the user folders at the profile, then starts the app. Known folders
// such as AppData are defined relative to %USERPROFILE%, so most apps follow.
func winLauncherArgs(profile, target, args string) string {
	set := func(k, v string) string { return fmt.Sprintf(`set "%s=%s"`, k, v) }
	return strings.TrimSpace("/c " + strings.Join([]string{
		set("USERPROFILE", profile),
		set("HOME", profile),
		set("APPDATA", profile+`\AppData\Roaming`),
		set("LOCALAPPDATA", profile+`\AppData\Local`),
		fmt.Sprintf(`start "" "%s" %s`, target, args),
	}, " & "))
}

// winScanScript prints the shortcuts found under paths (folders are searched
// recursively) as a JSON array.
func winScanScript(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = psQuote(p)
	}
	return `[Console]::OutputEncoding = [Text.Encoding]::UTF8
$sh = New-Object -ComObject WScript.Shell
$files = foreach ($p in @(` + strings.Join(quoted, ", ") + `)) {
  if (Test-Path -LiteralPath $p -PathType Container) { Get-ChildItem -LiteralPath $p -Recurse -Filter *.lnk -ErrorAction SilentlyContinue }
  elseif (Test-Path -LiteralPath $p) { Get-Item -LiteralPath $p }
}
$out = foreach ($f in $files) {
  $l = $sh.CreateShortcut($f.FullName)
  [pscustomobject]@{ Path = $f.FullName; Name = $f.BaseName; Target = $l.TargetPath; Args = $l.Arguments; Icon = $l.IconLocation }
}
ConvertTo-Json -Compress -InputObject @($out)`
}

// winShortcutScript creates a minimized-launch shortcut.
func winShortcutScript(dest, target, args, workdir, icon, desc string) string {
	return `$s = (New-Object -ComObject WScript.Shell).CreateShortcut(` + psQuote(dest) + `)
$s.TargetPath = ` + psQuote(target) + `
$s.Arguments = ` + psQuote(args) + `
$s.WorkingDirectory = ` + psQuote(workdir) + `
$s.IconLocation = ` + psQuote(icon) + `
$s.WindowStyle = 7
$s.Description = ` + psQuote(desc) + `
$s.Save()`
}

// winIconSource asks the shell for a file's 256px "jumbo" icon.
const winIconSource = `using System;
using System.Drawing;
using System.Runtime.InteropServices;
public static class TwinIcon {
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  struct SHFILEINFO {
    public IntPtr hIcon; public int iIcon; public uint dwAttributes;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 260)] public string szDisplayName;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 80)] public string szTypeName;
  }
  [DllImport("shell32.dll", CharSet = CharSet.Unicode)]
  static extern IntPtr SHGetFileInfo(string path, uint attrs, ref SHFILEINFO info, uint size, uint flags);
  [DllImport("shell32.dll")]
  static extern int SHGetImageList(int list, ref Guid iid, out IntPtr ppv);
  [DllImport("comctl32.dll")]
  static extern IntPtr ImageList_GetIcon(IntPtr list, int index, int flags);
  public static Bitmap Jumbo(string path) {
    var info = new SHFILEINFO();
    SHGetFileInfo(path, 0, ref info, (uint)Marshal.SizeOf(info), 0x4000); // SHGFI_SYSICONINDEX
    var iid = new Guid("46EB5926-582E-4017-9FDF-E8998DAA0950");          // IImageList
    IntPtr list;
    SHGetImageList(4, ref iid, out list);                                 // SHIL_JUMBO
    return Icon.FromHandle(ImageList_GetIcon(list, info.iIcon, 1)).ToBitmap();
  }
}`

// winIconScript saves file's icon as a PNG, falling back to the 32px
// associated icon when the jumbo one isn't available.
func winIconScript(file, out string) string {
	return `Add-Type -AssemblyName System.Drawing
Add-Type -ReferencedAssemblies System.Drawing -TypeDefinition @'
` + winIconSource + `
'@
try { $b = [TwinIcon]::Jumbo(` + psQuote(file) + `) }
catch { $b = ([System.Drawing.Icon]::ExtractAssociatedIcon(` + psQuote(file) + `)).ToBitmap() }
$b.Save(` + psQuote(out) + `, [System.Drawing.Imaging.ImageFormat]::Png)`
}

// iconLocation splits a shortcut's "path,index" icon location.
func iconLocation(loc string) string {
	if i := strings.LastIndex(loc, ","); i > 0 {
		loc = loc[:i]
	}
	return strings.TrimSpace(loc)
}

// encodeICO packs images into a .ico file using PNG-compressed entries,
// which Windows has read since Vista.
func encodeICO(w io.Writer, imgs []image.Image) error {
	var data [][]byte
	for _, img := range imgs {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return err
		}
		data = append(data, b.Bytes())
	}
	head := make([]byte, 6+16*len(imgs))
	binary.LittleEndian.PutUint16(head[2:], 1) // type: icon
	binary.LittleEndian.PutUint16(head[4:], uint16(len(imgs)))
	offset := len(head)
	for i, img := range imgs {
		e := head[6+16*i:]
		e[0], e[1] = byte(img.Bounds().Dx()%256), byte(img.Bounds().Dy()%256) // 256 is stored as 0
		binary.LittleEndian.PutUint16(e[4:], 1)                                // color planes
		binary.LittleEndian.PutUint16(e[6:], 32)                               // bits per pixel
		binary.LittleEndian.PutUint32(e[8:], uint32(len(data[i])))
		binary.LittleEndian.PutUint32(e[12:], uint32(offset))
		offset += len(data[i])
	}
	if _, err := w.Write(head); err != nil {
		return err
	}
	for _, d := range data {
		if _, err := w.Write(d); err != nil {
			return err
		}
	}
	return nil
}
