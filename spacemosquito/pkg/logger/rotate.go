package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingFile is an append-only log file that rotates by size: path is
// renamed to path.1 (path.1 to path.2, …) once it would exceed MaxBytes,
// keeping Keep old files.
type RotatingFile struct {
	path     string
	maxBytes int64
	keep     int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// OpenRotatingFile opens (or creates) path for appending, creating its
// directory.
func OpenRotatingFile(path string, maxBytes int64, keep int) (*RotatingFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &RotatingFile{path: path, maxBytes: maxBytes, keep: keep}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

// Write appends p, rotating first when the file would grow past MaxBytes.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maxBytes > 0 && r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *RotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	for i := r.keep; i >= 1; i-- {
		src := r.path
		if i > 1 {
			src = fmt.Sprintf("%s.%d", r.path, i-1)
		}
		_ = os.Rename(src, fmt.Sprintf("%s.%d", r.path, i)) // missing files are fine
	}
	if r.keep < 1 {
		_ = os.Remove(r.path)
	}
	return r.open()
}

// Sync flushes the file.
func (r *RotatingFile) Sync() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Sync()
}

// Close closes the file.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
