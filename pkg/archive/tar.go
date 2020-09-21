package archive

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

// TarOpts configures options for Create/Extract tar
type TarOpts func(*tarOpts)

// TODO: add support for compressed files with either gzip or bzip
type tarOpts struct{}

// Uncompressed option to tar (noop)
func Uncompressed(to *tarOpts) {
	return
}

// TODO: add option for full path or to adjust the relative path

// Tar creation
func Tar(ctx context.Context, path string, w io.Writer, opts ...TarOpts) error {
	to := tarOpts{}
	for _, opt := range opts {
		opt(&to)
	}

	tw := tar.NewWriter(w)
	defer tw.Close()

	// walk the path performing a recursive tar
	filepath.Walk(path, func(file string, fi os.FileInfo, err error) error {
		// return any errors filepath encounters accessing the file
		if err != nil {
			return err
		}

		// TODO: handle symlinks, security attributes, hard links
		// TODO: add options for file owner and timestamps

		// adjust for relative path
		relPath, err := filepath.Rel(path, file)
		if err != nil || relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(fi, relPath)
		if err != nil {
			return err
		}

		header.Format = tar.FormatPAX
		header.Name = filepath.ToSlash(relPath)
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.ModTime = header.ModTime.Truncate(time.Second)

		if err = tw.WriteHeader(header); err != nil {
			return err
		}

		// open file and copy contents into tar writer
		if header.Typeflag == tar.TypeReg && header.Size > 0 {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			if _, err = io.Copy(tw, f); err != nil {
				return err
			}
			f.Close()
		}

		return nil
	})
	return nil
}

// Extract Tar
func Extract(ctx context.Context, path string, r io.Reader, opts ...TarOpts) error {
	to := tarOpts{}
	for _, opt := range opts {
		opt(&to)
	}

	// TODO: verify path exists
	// TODO: decompress

	// TODO: implement tar extract method

	return ErrNotImplemented
}
