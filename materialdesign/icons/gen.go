// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/glycerine/shiny/iconvg"
	"golang.org/x/image/math/f32"
)

var mdicons = flag.String("mdicons", "", "The directory on the local file system where "+
	"https://github.com/google/material-design-icons was checked out",
)

// outSize is the width and height (in ideal vector space) of the generated
// IconVG graphic, regardless of the size of the input SVG.
const outSize = 48

// errSkip deliberately skips generating an icon.
//
// When manually debugging one particular icon, it can be useful to add
// something like:
// 	if baseName != "check_box" { return errSkip }
// at the top of func genFile.
var errSkip = errors.New("skipping SVG to IconVG conversion")

var (
	out      = new(bytes.Buffer)
	failures = []string{}
	varNames = []string{}

	totalFiles      int
	totalIVGBytes   int
	totalPNG24Bytes int
	totalPNG48Bytes int
	totalSVGBytes   int
)

var acronyms = map[string]string{
	"3d":            "3D",
	"ac":            "AC",
	"adb":           "ADB",
	"airplanemode":  "AirplaneMode",
	"atm":           "ATM",
	"av":            "AV",
	"ccw":           "CCW",
	"cw":            "CW",
	"din":           "DIN",
	"dns":           "DNS",
	"dvr":           "DVR",
	"eta":           "ETA",
	"ev":            "EV",
	"gif":           "GIF",
	"gps":           "GPS",
	"hd":            "HD",
	"hdmi":          "HDMI",
	"hdr":           "HDR",
	"http":          "HTTP",
	"https":         "HTTPS",
	"iphone":        "IPhone",
	"iso":           "ISO",
	"jpeg":          "JPEG",
	"markunread":    "MarkUnread",
	"mms":           "MMS",
	"nfc":           "NFC",
	"ondemand":      "OnDemand",
	"pdf":           "PDF",
	"phonelink":     "PhoneLink",
	"png":           "PNG",
	"rss":           "RSS",
	"rv":            "RV",
	"sd":            "SD",
	"sim":           "SIM",
	"sip":           "SIP",
	"sms":           "SMS",
	"streetview":    "StreetView",
	"svideo":        "SVideo",
	"textdirection": "TextDirection",
	"textsms":       "TextSMS",
	"timelapse":     "TimeLapse",
	"toc":           "TOC",
	"tv":            "TV",
	"usb":           "USB",
	"vpn":           "VPN",
	"wb":            "WB",
	"wc":            "WC",
	"whatshot":      "WhatsHot",
	"wifi":          "WiFi",
}

func upperCase(s string) string {
	if a, ok := acronyms[s]; ok {
		return a
	}
	if c := s[0]; 'a' <= c && c <= 'z' {
		return string(c-0x20) + s[1:]
	}
	return s
}

func main() {
	flag.Parse()

	out.WriteString("// generated by go run gen.go; DO NOT EDIT\n\npackage icons\n\n")

	f, err := os.Open(*mdicons)
	if err != nil {
		log.Fatalf("%v\n\nDid you override the -mdicons flag in icons.go?\n\n", err)
	}
	defer f.Close()
	infos, err := f.Readdir(-1)
	if err != nil {
		log.Fatal(err)
	}
	names := []string{}
	for _, info := range infos {
		if !info.IsDir() {
			continue
		}
		name := info.Name()
		if name[0] == '.' {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		genDir(name)
	}

	fmt.Fprintf(out,
		"// In total, %d SVG bytes in %d files (%d PNG bytes at 24px * 24px,\n"+
			"// %d PNG bytes at 48px * 48px) converted to %d IconVG bytes.\n",
		totalSVGBytes, totalFiles, totalPNG24Bytes, totalPNG48Bytes, totalIVGBytes)

	if len(failures) != 0 {
		out.WriteString("\n/*\nFAILURES:\n\n")
		for _, failure := range failures {
			out.WriteString(failure)
			out.WriteByte('\n')
		}
		out.WriteString("\n*/")
	}

	raw := out.Bytes()
	formatted, err := format.Source(raw)
	if err != nil {
		log.Fatalf("gofmt failed: %v\n\nGenerated code:\n%s", err, raw)
	}
	if err := ioutil.WriteFile("data.go", formatted, 0644); err != nil {
		log.Fatalf("WriteFile failed: %s\n", err)
	}

	// Generate data_test.go. The code immediately above generates data.go.
	{
		b := new(bytes.Buffer)
		b.WriteString("// generated by go run gen.go; DO NOT EDIT\n\npackage icons\n\n")
		b.WriteString("var list = []struct{ name string; data []byte } {\n")
		for _, v := range varNames {
			fmt.Fprintf(b, "{%q, %s},\n", v, v)
		}
		b.WriteString("}\n\n")
		raw := b.Bytes()
		formatted, err := format.Source(raw)
		if err != nil {
			log.Fatalf("gofmt failed: %v\n\nGenerated code:\n%s", err, raw)
		}
		if err := ioutil.WriteFile("data_test.go", formatted, 0644); err != nil {
			log.Fatalf("WriteFile failed: %s\n", err)
		}
	}
}

func genDir(dirName string) {
	fqPNGDirName := filepath.FromSlash(path.Join(*mdicons, dirName, "1x_web"))
	fqSVGDirName := filepath.FromSlash(path.Join(*mdicons, dirName, "svg/production"))
	f, err := os.Open(fqSVGDirName)
	if err != nil {
		return
	}
	defer f.Close()

	infos, err := f.Readdir(-1)
	if err != nil {
		log.Fatal(err)
	}
	baseNames, fileNames, sizes := []string{}, map[string]string{}, map[string]int{}
	for _, info := range infos {
		name := info.Name()

		if !strings.HasPrefix(name, "ic_") || skippedFiles[[2]string{dirName, name}] {
			continue
		}
		size := 0
		switch {
		case strings.HasSuffix(name, "_12px.svg"):
			size = 12
		case strings.HasSuffix(name, "_18px.svg"):
			size = 18
		case strings.HasSuffix(name, "_24px.svg"):
			size = 24
		case strings.HasSuffix(name, "_36px.svg"):
			size = 36
		case strings.HasSuffix(name, "_48px.svg"):
			size = 48
		default:
			continue
		}

		baseName := name[3 : len(name)-9]
		if prevSize, ok := sizes[baseName]; ok {
			if size > prevSize {
				fileNames[baseName] = name
				sizes[baseName] = size
			}
		} else {
			fileNames[baseName] = name
			sizes[baseName] = size
			baseNames = append(baseNames, baseName)
		}
	}

	sort.Strings(baseNames)
	for _, baseName := range baseNames {
		fileName := fileNames[baseName]
		err := genFile(fqSVGDirName, dirName, baseName, fileName, float32(sizes[baseName]))
		if err == errSkip {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%v/svg/production/%v: %v", dirName, fileName, err))
			continue
		}
		totalPNG24Bytes += pngSize(fqPNGDirName, dirName, baseName, 24)
		totalPNG48Bytes += pngSize(fqPNGDirName, dirName, baseName, 48)
	}
}

func pngSize(fqPNGDirName, dirName, baseName string, targetSize int) int {
	for _, size := range [...]int{48, 24, 18} {
		if size > targetSize {
			continue
		}
		fInfo, err := os.Stat(filepath.Join(fqPNGDirName,
			fmt.Sprintf("ic_%s_black_%ddp.png", baseName, size)))
		if err != nil {
			continue
		}
		return int(fInfo.Size())
	}
	failures = append(failures,
		fmt.Sprintf("no PNG found for %s/1x_web/ic_%s_black_{48,24,18}dp.png", dirName, baseName))
	return 0
}

type SVG struct {
	Width   float32 `xml:"where,attr"`
	Height  float32 `xml:"height,attr"`
	ViewBox string  `xml:"viewBox,attr"`
	Paths   []Path  `xml:"path"`
	// Some of the SVG files contain <circle> elements, not just <path>
	// elements. IconVG doesn't have circles per se. Instead, we convert such
	// circles to be paired arcTo commands, tacked on to the first path.
	//
	// In general, this isn't correct if the circles and the path overlap, but
	// that doesn't happen in the specific case of the Material Design icons.
	Circles []Circle `xml:"circle"`
}

type Path struct {
	D           string   `xml:"d,attr"`
	Fill        string   `xml:"fill,attr"`
	FillOpacity *float32 `xml:"fill-opacity,attr"`
	Opacity     *float32 `xml:"opacity,attr"`
}

type Circle struct {
	Cx float32 `xml:"cx,attr"`
	Cy float32 `xml:"cy,attr"`
	R  float32 `xml:"r,attr"`
}

var skippedPaths = map[string]string{
	// hardware/svg/production/ic_scanner_48px.svg contains a filled white
	// rectangle that is overwritten by the subsequent path.
	//
	// See https://github.com/google/material-design-icons/issues/490
	//
	// Matches <path fill="#fff" d="M16 34h22v4H16z"/>
	"M16 34h22v4H16z": "#fff",

	// device/svg/production/ic_airplanemode_active_48px.svg and
	// maps/svg/production/ic_flight_48px.svg contain a degenerate path that
	// contains only one moveTo op.
	//
	// See https://github.com/google/material-design-icons/issues/491
	//
	// Matches <path d="M20.36 18"/>
	"M20.36 18": "",
}

var skippedFiles = map[[2]string]bool{
	// ic_play_circle_filled_white_48px.svg is just the same as
	// ic_play_circle_filled_48px.svg with an explicit fill="#fff".
	{"av", "ic_play_circle_filled_white_48px.svg"}: true,
}

func genFile(fqSVGDirName, dirName, baseName, fileName string, size float32) error {
	fqFileName := filepath.Join(fqSVGDirName, fileName)
	svgData, err := ioutil.ReadFile(fqFileName)
	if err != nil {
		return err
	}

	varName := upperCase(dirName)
	for _, s := range strings.Split(baseName, "_") {
		varName += upperCase(s)
	}
	fmt.Fprintf(out, "var %s = []byte{", varName)
	defer fmt.Fprintf(out, "\n}\n\n")
	varNames = append(varNames, varName)

	var enc iconvg.Encoder
	enc.Reset(iconvg.Metadata{
		ViewBox: iconvg.Rectangle{
			Min: f32.Vec2{-24, -24},
			Max: f32.Vec2{+24, +24},
		},
		Palette: iconvg.DefaultPalette,
	})

	g := &SVG{}
	if err := xml.Unmarshal(svgData, g); err != nil {
		return err
	}

	var vbx, vby float32
	for i, v := range strings.Split(g.ViewBox, " ") {
		f, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return err
		}
		switch i {
		case 0:
			vbx = float32(f)
		case 1:
			vby = float32(f)
		}
	}
	offset := f32.Vec2{
		vbx * outSize / size,
		vby * outSize / size,
	}

	// adjs maps from opacity to a cReg adj value.
	adjs := map[float32]uint8{}

	for _, p := range g.Paths {
		if fill, ok := skippedPaths[p.D]; ok && fill == p.Fill {
			continue
		}
		if err := genPath(&enc, &p, adjs, size, offset, g.Circles); err != nil {
			return err
		}
		g.Circles = nil
	}

	if len(g.Circles) != 0 {
		if err := genPath(&enc, &Path{}, adjs, size, offset, g.Circles); err != nil {
			return err
		}
		g.Circles = nil
	}

	ivgData, err := enc.Bytes()
	if err != nil {
		return err
	}
	for i, x := range ivgData {
		if i&0x0f == 0x00 {
			out.WriteByte('\n')
		}
		fmt.Fprintf(out, "%#02x, ", x)
	}

	totalFiles++
	totalSVGBytes += len(svgData)
	totalIVGBytes += len(ivgData)
	return nil
}

func genPath(enc *iconvg.Encoder, p *Path, adjs map[float32]uint8, size float32, offset f32.Vec2, circles []Circle) error {
	adj := uint8(0)
	opacity := float32(1)
	if p.Opacity != nil {
		opacity = *p.Opacity
	} else if p.FillOpacity != nil {
		opacity = *p.FillOpacity
	}
	if opacity != 1 {
		var ok bool
		if adj, ok = adjs[opacity]; !ok {
			adj = uint8(len(adjs) + 1)
			adjs[opacity] = adj
			// Set CREG[0-adj] to be a blend of transparent (0x7f) and the
			// first custom palette color (0x80).
			enc.SetCReg(adj, false, iconvg.BlendColor(uint8(opacity*0xff), 0x7f, 0x80))
		}
	}

	needStartPath := true
	if p.D != "" {
		needStartPath = false
		if err := genPathData(enc, adj, p.D, size, offset); err != nil {
			return err
		}
	}

	for _, c := range circles {
		// Normalize.
		cx := c.Cx * outSize / size
		cx -= outSize/2 + offset[0]
		cy := c.Cy * outSize / size
		cy -= outSize/2 + offset[1]
		r := c.R * outSize / size

		if needStartPath {
			needStartPath = false
			enc.StartPath(adj, cx-r, cy)
		} else {
			enc.ClosePathAbsMoveTo(cx-r, cy)
		}

		// Convert a circle to two relative arcTo ops, each of 180 degrees.
		// We can't use one 360 degree arcTo as the start and end point
		// would be coincident and the computation is degenerate.
		enc.RelArcTo(r, r, 0, false, true, +2*r, 0)
		enc.RelArcTo(r, r, 0, false, true, -2*r, 0)
	}

	enc.ClosePathEndPath()
	return nil
}

func genPathData(enc *iconvg.Encoder, adj uint8, pathData string, size float32, offset f32.Vec2) error {
	if strings.HasSuffix(pathData, "z") {
		pathData = pathData[:len(pathData)-1]
	}
	r := strings.NewReader(pathData)

	var args [6]float32
	op, relative, started := byte(0), false, false
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch {
		case b == ' ':
			continue
		case 'A' <= b && b <= 'Z':
			op, relative = b, false
		case 'a' <= b && b <= 'z':
			op, relative = b, true
		default:
			r.UnreadByte()
		}

		n := 0
		switch op {
		case 'L', 'l', 'T', 't':
			n = 2
		case 'Q', 'q', 'S', 's':
			n = 4
		case 'C', 'c':
			n = 6
		case 'H', 'h', 'V', 'v':
			n = 1
		case 'M', 'm':
			n = 2
		case 'Z', 'z':
		default:
			return fmt.Errorf("unknown opcode %c\n", b)
		}

		scan(&args, r, n)
		normalize(&args, n, op, size, offset, relative)

		switch op {
		case 'L':
			enc.AbsLineTo(args[0], args[1])
		case 'l':
			enc.RelLineTo(args[0], args[1])
		case 'T':
			enc.AbsSmoothQuadTo(args[0], args[1])
		case 't':
			enc.RelSmoothQuadTo(args[0], args[1])
		case 'Q':
			enc.AbsQuadTo(args[0], args[1], args[2], args[3])
		case 'q':
			enc.RelQuadTo(args[0], args[1], args[2], args[3])
		case 'S':
			enc.AbsSmoothCubeTo(args[0], args[1], args[2], args[3])
		case 's':
			enc.RelSmoothCubeTo(args[0], args[1], args[2], args[3])
		case 'C':
			enc.AbsCubeTo(args[0], args[1], args[2], args[3], args[4], args[5])
		case 'c':
			enc.RelCubeTo(args[0], args[1], args[2], args[3], args[4], args[5])
		case 'H':
			enc.AbsHLineTo(args[0])
		case 'h':
			enc.RelHLineTo(args[0])
		case 'V':
			enc.AbsVLineTo(args[0])
		case 'v':
			enc.RelVLineTo(args[0])
		case 'M':
			if !started {
				started = true
				enc.StartPath(adj, args[0], args[1])
			} else {
				enc.ClosePathAbsMoveTo(args[0], args[1])
			}
		case 'm':
			enc.ClosePathRelMoveTo(args[0], args[1])
		}
	}
	return nil
}

func scan(args *[6]float32, r *strings.Reader, n int) {
	for i := 0; i < n; i++ {
		for {
			if b, _ := r.ReadByte(); b != ' ' {
				r.UnreadByte()
				break
			}
		}
		fmt.Fscanf(r, "%f", &args[i])
	}
}

func atof(s []byte) (float32, error) {
	f, err := strconv.ParseFloat(string(s), 32)
	if err != nil {
		return 0, fmt.Errorf("could not parse %q as a float32: %v", s, err)
	}
	return float32(f), err
}

func normalize(args *[6]float32, n int, op byte, size float32, offset f32.Vec2, relative bool) {
	for i := 0; i < n; i++ {
		args[i] *= outSize / size
		if relative {
			continue
		}
		args[i] -= outSize / 2
		switch {
		case n != 1:
			args[i] -= offset[i&0x01]
		case op == 'H':
			args[i] -= offset[0]
		case op == 'V':
			args[i] -= offset[1]
		}
	}
}
