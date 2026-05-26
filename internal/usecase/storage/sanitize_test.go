package storage

import "testing"

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "file.bin"},
		{"only whitespace", "   ", "file.bin"},
		{"dot", ".", "file.bin"},
		{"double dot", "..", "file.bin"},

		// path traversal — only the base name must survive
		{"unix traversal", "../../etc/passwd", "passwd"},
		{"unix deep traversal", "/var/log/secret.txt", "secret.txt"},

		// Windows path separators sent by browser clients — stripped to base name
		{"windows backslash", "C:\\Users\\alice\\document.pdf", "document.pdf"},
		{"windows UNC path", "\\\\server\\share\\file.txt", "file.txt"},

		// normal cases
		{"plain name", "report.pdf", "report.pdf"},
		{"name with spaces", "  my file.txt  ", "my file.txt"},
		{"no extension", "README", "README"},
		{"dotfile", ".gitignore", ".gitignore"},
		{"unicode", "привет мир.docx", "привет мир.docx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeFileName(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeFileName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
