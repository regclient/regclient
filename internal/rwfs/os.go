package rwfs

import (
	"io/fs"
	"os"
	"path"
)

// RWFS implemented for the os filesystem
// TODO: support fs.DirEntry, fs.ReadDirFile, fs.StatFS

type OSFS struct {
	dir string
}

// at present, this is just a pass through
type OSFile struct {
	*os.File
}

func OSNew(base string) *OSFS {
	if base == "" || base == "." {
		return &OSFS{dir: base}
	}
	base = path.Clean(base)
	return &OSFS{
		dir: base,
	}
}

func (o *OSFS) Create(name string) (WFile, error) {
	file, err := o.join("create", name)
	if err != nil {
		return nil, err
	}
	fh, err := os.Create(file)
	if err != nil {
		return nil, err
	}
	return &OSFile{
		File: fh,
	}, nil
}

func (o *OSFS) Mkdir(name string, perm fs.FileMode) error {
	if name == "." {
		return fs.ErrExist
	}
	dir, err := o.join("mkdir", name)
	if err != nil {
		return err
	}
	return os.Mkdir(dir, perm)
}

func (o *OSFS) OpenFile(name string, flag int, perm fs.FileMode) (RWFile, error) {
	file, err := o.join("open", name)
	if err != nil {
		return nil, err
	}
	fh, err := os.OpenFile(file, flag, perm)
	if err != nil {
		return nil, err
	}
	return &OSFile{
		File: fh,
	}, nil
}

func (o *OSFS) Open(name string) (fs.File, error) {
	file, err := o.join("open", name)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	return &OSFile{
		File: fh,
	}, nil
}

func (o *OSFS) Sub(name string) (*OSFS, error) {
	if name == "." {
		return o, nil
	}
	full, err := o.join("sub", name)
	if err != nil {
		return nil, err
	}
	return &OSFS{
		dir: full,
	}, nil
}

func (o *OSFS) join(op, name string) (string, error) {
	if name == "" || name == "." {
		if o.dir != "" {
			return o.dir, nil
		}
		return ".", nil
	}
	if !fs.ValidPath(name) {
		return "", &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if name[:1] == "/" {
		name = path.Clean(name)
	} else {
		// for relative paths, clean with a preceding "/" to strip all leading ".."
		// and then turn back into a relative path
		name = path.Clean("/" + name)
		name = name[1:]
	}
	return path.Join(o.dir, name), nil
}
