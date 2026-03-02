package rutracker

import (
	"io"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// decodeWindows1251 converts a Windows-1251 encoded reader to UTF-8.
func decodeWindows1251(r io.Reader) io.Reader {
	return transform.NewReader(r, charmap.Windows1251.NewDecoder())
}
