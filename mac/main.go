package main

import (
	"bytes"
	"compress/zlib"
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
)

// atomicWriteFile writes data to a temp file in the same directory, then
// renames it over the target. This bypasses macOS SIP/TCC restrictions.
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
	binaryPath := flag.String("binary", "", "Path to Max binary (e.g. Max.app/Contents/MacOS/Max)")
	styleName := flag.String("style", "", "Style name to patch (e.g. MarkdownMessageBase)")
	pixelSize := flag.Int("size", 0, "New font_size value")
	lineHeight := flag.Int("line-height", 0, "New line_height value (optional)")
	noSign := flag.Bool("no-sign", false, "Skip ad-hoc code signing after patching")
	noBackup := flag.Bool("no-backup", false, "Skip creating a .bak backup of the original binary")
	flag.Parse()

	if *binaryPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: font-patcher -binary <path> -style <name> -size <px>")
		fmt.Fprintln(os.Stderr, "\nExamples:")
		fmt.Fprintln(os.Stderr, "  font-patcher -binary Max.app/Contents/MacOS/Max -style MarkdownMessageBase -size 12")
		os.Exit(1)
	}

	data, err := os.ReadFile(*binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading binary: %v\n", err)
		os.Exit(1)
	}

	if !*noBackup && *styleName != "" {
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

	patched := 0
	pos := 0

	for pos < len(data)-10 {
		// Fast-forward to next possible zlib header
		idx := bytes.Index(data[pos:], []byte{0x78})
		if idx == -1 {
			break
		}
		idx += pos
		pos = idx + 1 // Advance for next iteration

		if idx+2 > len(data) {
			break
		}
		
		header := data[idx : idx+2]
		if header[1] != 0x9c && header[1] != 0x5e && header[1] != 0xda {
			continue // Not a standard zlib header
		}

		// Try to decompress assuming it's a zlib stream
		reader, err := zlib.NewReader(bytes.NewReader(data[idx:]))
		if err != nil {
			continue
		}
		var decompBuf bytes.Buffer
		// Limit to 1MB to prevent OOM
		_, err = io.CopyN(&decompBuf, reader, 1024*1024)
		if err != nil && err != io.EOF {
			reader.Close()
			continue
		}
		reader.Close()

		decomp := decompBuf.Bytes()
		
		// Check if it's our target JSON (must contain Hero and either the target style or "Hero" if listing)
		if !bytes.Contains(decomp, []byte(`"Hero"`)) {
			continue
		}
		if *styleName != "list" {
			foundAny := false
			for _, ts := range strings.Split(*styleName, ",") {
				if bytes.Contains(decomp, []byte(`"`+strings.TrimSpace(ts)+`"`)) {
					foundAny = true
					break
				}
			}
			if !foundAny {
				continue
			}
		}
		if bytes.Contains(decomp, []byte("Typography.FontUtil")) {
			continue // This is Typography.qml, not the JSON
		}

		// Found the Qt resource typography.json!
		if idx < 8 {
			continue // Safety check, header offsets won't work
		}

		// Read Qt resource header lengths
		compLenWithUncomp := binary.BigEndian.Uint32(data[idx-8 : idx-4])
		origCompLen := compLenWithUncomp - 4
		uncompLen := binary.BigEndian.Uint32(data[idx-4 : idx])

		if uncompLen != uint32(len(decomp)) {
			continue // Not a valid Qt resource block header
		}

		fmt.Printf("\nFound JSON table at offset 0x%x\n", idx)

		// We use string replacement to preserve map key order and exact formatting.
		// Unmarshaling to map[string]interface{} sorts keys alphabetically, which destroys
		// zlib's original back-references and inflates the compressed size.
		
		sDecomp := string(decomp)
		
		if *styleName == "list" {
			var root map[string]map[string]map[string]interface{}
			
			// Qt QJsonDocument allows trailing commas, but Go's encoding/json forbids them.
			// The previous regex replacement may have caused trailing commas.
			decompStr := strings.ReplaceAll(string(decomp), ",}", "}")
			decompStr = strings.ReplaceAll(decompStr, ",\n}", "\n}")
			
			if err := json.Unmarshal([]byte(decompStr), &root); err == nil {
				fmt.Printf("\nAvailable typography styles and current values:\n")
				for styleName, variants := range root {
					// Grab the 'Large' variant or any available to show representative sizes
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
							styleName, rep["font_size"], rep["line_height"], rep["weight"])
					} else {
						fmt.Printf("  - %s (no sizes found)\n", styleName)
					}
				}
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, "error parsing JSON for list: %v\n", err)
			}
			continue
		}
		

		// Strip ALL whitespace from the ENTIRE JSON BEFORE processing.
		// Doing this guarantees Go's zlib output will be smaller than the original C zlib output,
		// and normalizes the format so we can safely index strings.
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
			
			// Find the block for the requested style
			reMarker := regexp.MustCompile(`"` + targetStyle + `":\{`)
			loc := reMarker.FindStringIndex(sDecomp)
			if loc == nil {
				fmt.Fprintf(os.Stderr, "style %s not found in JSON\n", targetStyle)
				continue
			}
			styleStart := loc[0]
			
			// Find the end of this style block (next top-level style starts after }}," or file ends at }}})
			styleEnd := strings.Index(sDecomp[styleStart:], `}},"`)
			if styleEnd == -1 {
				// Try end of file marker
				styleEnd = strings.Index(sDecomp[styleStart:], `}}}`)
				if styleEnd == -1 {
					styleEnd = len(sDecomp) - styleStart // Fallback to end
				} else {
					styleEnd += 2
				}
			} else {
				styleEnd += 2
			}
			styleEnd += styleStart
			
			styleBlock := sDecomp[styleStart:styleEnd]
			
			// Replace font_size
			reFontSize := regexp.MustCompile(`"font_size":\d+`)
			newStyleBlock := reFontSize.ReplaceAllString(styleBlock, fmt.Sprintf(`"font_size":%d`, *pixelSize))
			
			// Replace line_height if provided
			if *lineHeight > 0 {
				reLineHeight := regexp.MustCompile(`"line_height":(?:\d+|\d+\.\d+)`)
				newStyleBlock = reLineHeight.ReplaceAllString(newStyleBlock, fmt.Sprintf(`"line_height":%d`, *lineHeight))
			}
			
			// Apply modifications for this target style
			sDecomp = sDecomp[:styleStart] + newStyleBlock + sDecomp[styleEnd:]
		}

		newDecomp := []byte(sDecomp)

		// Recompress with highest compression
		var newCompBuf bytes.Buffer
		writer, _ := zlib.NewWriterLevel(&newCompBuf, zlib.BestCompression)
		writer.Write(newDecomp)
		writer.Close()

		newComp := newCompBuf.Bytes()

		if uint32(len(newComp)) > origCompLen {
			fmt.Fprintf(os.Stderr, "error: patched JSON is too large (new: %d, orig: %d).\n", len(newComp), origCompLen)
			fmt.Fprintf(os.Stderr, "Cannot patch in-place because it would shift other resources.\n")
			continue
		}

		// Success! Write the new chunk
		fmt.Printf("  compressed size: orig %d bytes -> new %d bytes\n", origCompLen, len(newComp))
		
		// Update headers
		binary.BigEndian.PutUint32(data[idx-8:idx-4], uint32(len(newComp))+4)
		binary.BigEndian.PutUint32(data[idx-4:idx], uint32(len(newDecomp)))

		// Overwrite old chunk with new chunk
		copy(data[idx:], newComp)
		
		// Pad remaining space of the original chunk with zeros
		for k := idx + len(newComp); k < idx+int(origCompLen); k++ {
			data[k] = 0
		}
		
		patched++
		pos = idx + int(origCompLen) // skip past this replaced chunk
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

	// Re-sign
	if !*noSign {
		appPath := *binaryPath
		for i := 0; i < 3; i++ {
			if idx := strings.LastIndex(appPath, "/"); idx >= 0 {
				appPath = appPath[:idx]
			}
		}
		if strings.HasSuffix(appPath, ".app") {
			fmt.Printf("Re-signing %s ...\n", appPath)
			cmd := exec.Command("codesign", "--force", "--deep", "--sign", "-", appPath)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: codesign failed: %v\n", err)
				fmt.Fprintf(os.Stderr, "Run manually: codesign --force --deep --sign - %s\n", appPath)
			} else {
				fmt.Println("Signed.")
			}
		}
	}
}
