package rwfs

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
)

const (
	// exactly one of these must be used
	O_RDONLY = os.O_RDONLY // read-only
	O_WRONLY = os.O_WRONLY // write-only
	O_RDWR   = os.O_RDWR   // read-write
	// remaining values may be or'ed
	O_APPEND = os.O_APPEND // append when writing
	O_CREATE = os.O_CREATE // create if missing
	O_EXCL   = os.O_EXCL   // file must not exist, used with O_CREATE
	O_SYNC   = os.O_SYNC   // synchronous I/O
	O_TRUNC  = os.O_TRUNC  // truncate on open
)

type RWFS interface {
	fs.FS
	WriteFS
}
type RWFile interface {
	fs.File
	WFile
}

type WriteFS interface {
	// Create creates a new file
	Create(string) (WFile, error)
	// Mkdir creates a directory
	Mkdir(string, fs.FileMode) error
	// OpenFile generalized file open with options for a flag and permissions
	OpenFile(string, int, fs.FileMode) (RWFile, error)
}

type WFile interface {
	// Close closes the open file
	Close() error
	// Stat returns the FileInfo of the file
	Stat() (fi fs.FileInfo, err error)
	// Write writes len(b) bytes to the file.
	// It returns the number of bytes written, and any error if n != len(b).
	Write(b []byte) (n int, err error)
}

func MkdirAll(rwfs RWFS, name string, perm fs.FileMode) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		// assemble directory up to this point
		cur := path.Join(parts[:i+1]...)
		fi, err := Stat(rwfs, cur)
		if errors.Is(err, fs.ErrNotExist) {
			// missing, create
			err := rwfs.Mkdir(cur, perm)
			if err != nil {
				return &fs.PathError{
					Op:   "mkdir",
					Path: cur,
					Err:  err,
				}
			}
		} else if err != nil {
			// unknown errors
			return &fs.PathError{
				Op:   "mkdir",
				Path: cur,
				Err:  err,
			}
		} else if !fi.IsDir() {
			// can't mkdir on existing file
			return &fs.PathError{
				Op:   "mkdir",
				Path: cur,
				Err:  fs.ErrExist,
			}
		}
		// exists and is a directory, next
	}
	return nil
}

func Stat(rfs fs.FS, name string) (fs.FileInfo, error) {
	fh, err := rfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	fi, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	return fi, nil
}

func ReadFile(rfs fs.FS, name string) ([]byte, error) {
	return fs.ReadFile(rfs, name)
}

func WriteFile(wfs WriteFS, name string, data []byte, perm fs.FileMode) error {
	// replace os flags?
	f, err := wfs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err1 := f.Close(); err1 != nil && err == nil {
		return err1
	}
	return err
}

func flagMode(flags int) int {
	return flags & (O_RDONLY | O_WRONLY | O_RDWR)
}

func flagSet(flag, flags int) bool {
	return (flags & flag) != 0
}
