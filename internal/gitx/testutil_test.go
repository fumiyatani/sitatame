package gitx

import "os"

func writeFileAt(p string, body []byte) error {
	return os.WriteFile(p, body, 0o644)
}

func mkdirAll(p string) error { return os.MkdirAll(p, 0o755) }
