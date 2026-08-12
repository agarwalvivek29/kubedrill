package provision

import "bytes"

// splitYAML splits a multi-document YAML file on `---` separators into
// individual document byte slices.
func splitYAML(raw []byte) [][]byte {
	// Normalize line endings, then split on lines that are exactly "---".
	parts := bytes.Split(raw, []byte("\n---"))
	out := make([][]byte, 0, len(parts))
	for _, p := range parts {
		out = append(out, bytes.TrimSpace(p))
	}
	return out
}
