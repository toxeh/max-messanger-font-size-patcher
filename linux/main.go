package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// atomicWriteFile writes data to a temp file in the same directory, then
// renames it over the target.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".font-patcher-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpName, path, err)
	}
	tmpName = "" // prevent cleanup
	return nil
}

func main() {
	binaryPath := flag.String("binary", "", "Path to max binary (e.g. /usr/share/max/bin/max)")
	styleName := flag.String("style", "", "Style name to patch (e.g. MarkdownMessageBase)")
	pixelSize := flag.Int("size", 0, "New font_size value")
	lineHeight := flag.Int("line-height", 0, "New line_height value (optional)")
	noBackup := flag.Bool("no-backup", false, "Skip creating a .bak backup of the original binary")
	flag.Parse()

	if *binaryPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: font-patcher -binary <path> -style <name> -size <px>")
		fmt.Fprintln(os.Stderr, "\nExamples:")
		fmt.Fprintln(os.Stderr, "  sudo font-patcher -binary /usr/share/max/bin/max -style MarkdownMessageBase -size 12")
		fmt.Fprintln(os.Stderr, "  font-patcher -binary /usr/share/max/bin/max -style list")
		os.Exit(1)
	}

	data, err := os.ReadFile(*binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading binary: %v\n", err)
		os.Exit(1)
	}

	if !*noBackup && *styleName != "" && *styleName != "list" {
		bakPath := *binaryPath + ".bak"
		if _, err := os.Stat(bakPath); os.IsNotExist(err) {
			if err := os.WriteFile(bakPath, data, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "error: creating backup: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Backup: %s\n", bakPath)
		} else {
			fmt.Printf("Backup already exists: %s\n", bakPath)
		}
	}

	if *styleName != "list" && *pixelSize <= 0 {
		fmt.Fprintln(os.Stderr, "error: -size must be positive")
		os.Exit(1)
	}

	// Qt6 on Linux uses zstd compression for resources (magic: 0x28 0xB5 0x2F 0xFD)
	// Qt RCC v3 header: 4 bytes (BE uint32) = compressed size, immediately before zstd magic.
	// No uncompressed length field in the header (unlike zlib-based RCC v1/v2).
	zstdMagic := []byte{0x28, 0xB5, 0x2F, 0xFD}

	patched := 0
	pos := 0

	for pos < len(data)-10 {
		idx := bytes.Index(data[pos:], zstdMagic)
		if idx == -1 {
			break
		}
		idx += pos
		pos = idx + 1

		if idx < 4 {
			continue
		}

		// Read the compressed length from the 4 bytes before zstd magic
		origCompLen := binary.BigEndian.Uint32(data[idx-4 : idx])
		if origCompLen == 0 || origCompLen > 1024*1024 {
			continue // Sanity check
		}

		// Try to decompress
		decoder, err := zstd.NewReader(bytes.NewReader(data[idx : idx+int(origCompLen)]))
		if err != nil {
			continue
		}
		var decompBuf bytes.Buffer
		_, err = io.Copy(&decompBuf, io.LimitReader(decoder, 1024*1024))
		decoder.Close()
		if err != nil {
			continue
		}

		decomp := decompBuf.Bytes()
		if len(decomp) < 50 {
			continue
		}

		// Check if it's our target JSON
		if !bytes.Contains(decomp, []byte(`"Hero"`)) || !bytes.Contains(decomp, []byte(`"font_size"`)) {
			continue
		}
		if bytes.Contains(decomp, []byte("Typography.FontUtil")) {
			continue
		}

		fmt.Printf("\nFound JSON table at offset 0x%x\n", idx)

		sDecomp := string(decomp)

		if *styleName == "list" {
			var root map[string]map[string]map[string]interface{}

			decompStr := strings.ReplaceAll(string(decomp), ",}", "}")
			decompStr = strings.ReplaceAll(decompStr, ",\n}", "\n}")

			if err := json.Unmarshal([]byte(decompStr), &root); err == nil {
				fmt.Printf("\nAvailable typography styles and current values:\n")
				for sn, variants := range root {
					var rep map[string]interface{}
					if large, ok := variants["Large"]; ok {
						rep = large
					} else {
						for _, v := range variants {
							rep = v
							break
						}
					}

					if rep != nil {
						fmt.Printf("  - %-26s px=%-2v lh=%-2v wt=%v\n",
							sn, rep["font_size"], rep["line_height"], rep["weight"])
					} else {
						fmt.Printf("  - %s (no sizes found)\n", sn)
					}
				}
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, "error parsing JSON for list: %v\n", err)
			}
			continue
		}

		// Strip ALL whitespace
		sDecomp = strings.ReplaceAll(sDecomp, " ", "")
		sDecomp = strings.ReplaceAll(sDecomp, "\n", "")
		sDecomp = strings.ReplaceAll(sDecomp, "\r", "")
		sDecomp = strings.ReplaceAll(sDecomp, "\t", "")

		// Remove the 'ax5' accessibility size variant entirely from all styles to save space
		reAx5 := regexp.MustCompile(`"ax5":\{[^}]*\},?`)
		sDecomp = reAx5.ReplaceAllString(sDecomp, "")
		sDecomp = strings.ReplaceAll(sDecomp, ",}", "}")

		stylesToPatch := strings.Split(*styleName, ",")

		for _, targetStyle := range stylesToPatch {
			targetStyle = strings.TrimSpace(targetStyle)

			reMarker := regexp.MustCompile(`"` + targetStyle + `":\{`)
			loc := reMarker.FindStringIndex(sDecomp)
			if loc == nil {
				fmt.Fprintf(os.Stderr, "style %s not found in JSON\n", targetStyle)
				continue
			}
			styleStart := loc[0]

			styleEnd := strings.Index(sDecomp[styleStart:], `}},"`)
			if styleEnd == -1 {
				styleEnd = strings.Index(sDecomp[styleStart:], `}}}`)
				if styleEnd == -1 {
					styleEnd = len(sDecomp) - styleStart
				} else {
					styleEnd += 2
				}
			} else {
				styleEnd += 2
			}
			styleEnd += styleStart

			styleBlock := sDecomp[styleStart:styleEnd]

			reFontSize := regexp.MustCompile(`"font_size":\d+`)
			newStyleBlock := reFontSize.ReplaceAllString(styleBlock, fmt.Sprintf(`"font_size":%d`, *pixelSize))

			if *lineHeight > 0 {
				reLineHeight := regexp.MustCompile(`"line_height":(?:\d+|\d+\.\d+)`)
				newStyleBlock = reLineHeight.ReplaceAllString(newStyleBlock, fmt.Sprintf(`"line_height":%d`, *lineHeight))
			}

			sDecomp = sDecomp[:styleStart] + newStyleBlock + sDecomp[styleEnd:]
		}

		newDecomp := []byte(sDecomp)

		// Recompress with zstd — try system zstd at level 22 (ultra) first,
		// fall back to Go library (max level 11) if unavailable.
		var newComp []byte
		if zstdPath, err := exec.LookPath("zstd"); err == nil {
			cmd := exec.Command(zstdPath, "-22", "--ultra", "--no-check", "-c")
			cmd.Stdin = bytes.NewReader(newDecomp)
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil {
				newComp = out.Bytes()
			}
		}
		if newComp == nil {
			// Fallback to Go library
			encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: creating zstd encoder: %v\n", err)
				continue
			}
			newComp = encoder.EncodeAll(newDecomp, nil)
			encoder.Close()
		}

		if uint32(len(newComp)) > origCompLen {
			fmt.Fprintf(os.Stderr, "error: patched JSON is too large (new: %d, orig: %d).\n", len(newComp), origCompLen)
			fmt.Fprintf(os.Stderr, "Cannot patch in-place because it would shift other resources.\n")
			continue
		}

		fmt.Printf("  compressed size: orig %d bytes -> new %d bytes\n", origCompLen, len(newComp))

		// Update the compressed length header (4 bytes before zstd magic)
		binary.BigEndian.PutUint32(data[idx-4:idx], uint32(len(newComp)))

		// Overwrite old chunk with new chunk
		copy(data[idx:], newComp)

		// Pad remaining space with zeros
		for k := idx + len(newComp); k < idx+int(origCompLen); k++ {
			data[k] = 0
		}

		patched++
		pos = idx + int(origCompLen)
	}

	if patched == 0 {
		fmt.Fprintln(os.Stderr, "error: no tables were patched. Could not find or compress typography.json.")
		os.Exit(1)
	}

	if err := atomicWriteFile(*binaryPath, data, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing binary: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nBinary written (%d resource chunk(s) patched).\n", patched)
}
