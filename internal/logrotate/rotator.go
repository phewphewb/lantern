package logrotate

import (
	"os"
)

// RotatingFile implements io.Writer. It checks the current file size before
// each write; when the file exceeds maxBytes it is renamed to "<path>.1"
// and a new file is opened. Only one rotated file is kept.
type RotatingFile struct {
	path     string
	maxBytes int64
	f        *os.File
}

func New(path string, maxBytes int64) *RotatingFile {
	return &RotatingFile{path: path, maxBytes: maxBytes}
}

func (r *RotatingFile) Write(p []byte) (int, error) {
	if err := r.openIfNeeded(); err != nil {
		return 0, err
	}
	if err := r.rotateIfNeeded(); err != nil {
		return 0, err
	}
	return r.f.Write(p)
}

func (r *RotatingFile) openIfNeeded() error {
	if r.f != nil {
		return nil
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	r.f = f
	return nil
}

func (r *RotatingFile) rotateIfNeeded() error {
	info, err := r.f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < r.maxBytes {
		return nil
	}
	// Close current file, rename to .1, reopen fresh.
	r.f.Close()
	r.f = nil
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return err
	}
	return r.openIfNeeded()
}
