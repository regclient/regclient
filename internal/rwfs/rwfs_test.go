package rwfs

import (
	"bytes"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func testRWFS(t *testing.T, rwfs RWFS) {
	exRootFile := "root.txt"
	exRootTxt := []byte("hello root")
	exSubDir := "new-dir"
	exSubFile1 := path.Join(exSubDir, "file1.txt")
	exSubTxt1 := []byte("hello sub 1")
	exSubFile2 := path.Join(exSubDir, "file2.txt")
	exSubTxt2 := []byte("hello sub 2")
	exSubFile3 := path.Join(exSubDir, "file3.txt")
	exSubTxt3 := []byte("hello sub 3")
	exNestedDir := "nested/dir"
	exNestedFile := "nested/dir/file"
	exNestedChildDir := "nested/dir/tree"
	exNestedTxt1 := []byte("hello nested take 1")
	exNestedTxt2 := []byte("hello nested take 2")

	// open current dir
	t.Run("curdir", func(t *testing.T) {
		fh, err := rwfs.Open(".")
		if err != nil {
			t.Errorf("open: %v", err)
			return
		}
		fi, err := fh.Stat()
		if err != nil {
			t.Errorf("stat: %v", err)
		}
		if !fi.IsDir() {
			t.Errorf("is not a directory")
		}
		b, err := io.ReadAll(fh)
		if err == nil {
			t.Errorf("readall on directory succeeded")
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close: %v", err)
		}
		if len(b) > 0 {
			t.Errorf("read contents: %s", string(b))
		}
	})

	// create subdirs
	t.Run("mkdir", func(t *testing.T) {
		err := rwfs.Mkdir(exSubDir, 0777)
		if err != nil {
			t.Errorf("mkdir: %v", err)
		}
		err = MkdirAll(rwfs, exNestedChildDir, 0777)
		if err != nil {
			t.Errorf("mkdirAll: %v", err)
		}
		err = rwfs.Mkdir("missing/missing-parent", 0777)
		if err == nil {
			t.Errorf("mkdir missing did not fail")
		}
	})

	// create files in root and subdirs
	// recreate file that already exists
	// create file with name of subdir, verify error
	// create file in non-existent directory, verify error
	// test WriteFile
	t.Run("create", func(t *testing.T) {
		// create a file in the root dir
		fh, err := rwfs.Create(exRootFile)
		if err != nil {
			t.Errorf("create %s: %v", exRootFile, err)
			return
		}
		i, err := fh.Write(exRootTxt)
		if err != nil {
			t.Errorf("write %s: %v", exRootFile, err)
		}
		if i != len(exRootTxt) {
			t.Errorf("write %s len, expected %d, received %d", exRootFile, len(exRootTxt), i)
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close %s: %v", exRootFile, err)
		}

		// create a file one level down
		fh, err = rwfs.Create(exSubFile1)
		if err != nil {
			t.Errorf("create %s: %v", exSubFile1, err)
			return
		}
		i, err = fh.Write(exSubTxt1)
		if err != nil {
			t.Errorf("write %s: %v", exSubFile1, err)
		}
		if i != len(exSubTxt1) {
			t.Errorf("write %s len, expected %d, received %d", exSubFile1, len(exSubTxt1), i)
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close %s: %v", exSubFile1, err)
		}

		// create a file more than one level down
		fh, err = rwfs.Create(exNestedFile)
		if err != nil {
			t.Errorf("create %s: %v", exNestedFile, err)
			return
		}
		i, err = fh.Write(exNestedTxt1)
		if err != nil {
			t.Errorf("write %s: %v", exNestedFile, err)
		}
		if i != len(exNestedTxt1) {
			t.Errorf("write %s len, expected %d, received %d", exNestedFile, len(exNestedTxt1), i)
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close %s: %v", exNestedFile, err)
		}

		// create a file in a missing directory
		_, err = rwfs.Create("missing/file.txt")
		if err == nil {
			t.Errorf("file created in missing directory")
		}

		// create a file with same name as a directory
		_, err = rwfs.Create(exSubDir)
		if err == nil {
			t.Errorf("file created on existing directory name")
		}

		// recreate a file that exists
		fh, err = rwfs.Create(exNestedFile)
		if err != nil {
			t.Errorf("create again %s: %v", exNestedFile, err)
			return
		}
		i, err = fh.Write(exNestedTxt2)
		if err != nil {
			t.Errorf("write again %s: %v", exNestedFile, err)
		}
		if i != len(exNestedTxt2) {
			t.Errorf("write again %s len, expected %d, received %d", exNestedFile, len(exNestedTxt2), i)
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close again %s: %v", exNestedFile, err)
		}

		// writefile
		err = WriteFile(rwfs, exSubFile2, exSubTxt2, 0666)
		if err != nil {
			t.Errorf("writeFile: %v", err)
		}
	})

	// read files in root and subdir
	t.Run("open", func(t *testing.T) {
		// open, read, and cmp contents
		fh, err := rwfs.Open(exRootFile)
		if err != nil {
			t.Errorf("open %s: %v", exRootFile, err)
			return
		}
		b, err := io.ReadAll(fh)
		if err != nil {
			t.Errorf("readall %s: %v", exRootFile, err)
			return
		}
		if !bytes.Equal(exRootTxt, b) {
			t.Errorf("contents mismatch %s, expected %s, received %s", exRootFile, string(exRootTxt), string(b))
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close %s: %v", exRootFile, err)
		}

		// verify ReadFile works
		b, err = ReadFile(rwfs, exNestedFile)
		if err != nil {
			t.Errorf("readfile %s: %v", exNestedFile, err)
			return
		}
		if !bytes.Equal(exNestedTxt2, b) {
			t.Errorf("contents mismatch %s, expected %s, received %s", exNestedFile, string(exNestedTxt2), string(b))
		}

		// verify open with a subdir
		fh, err = rwfs.Open(exSubFile1)
		if err != nil {
			t.Errorf("open %s: %v", exSubFile1, err)
			return
		}
		b, err = io.ReadAll(fh)
		if err != nil {
			t.Errorf("readall %s: %v", exSubFile1, err)
			return
		}
		if !bytes.Equal(exSubTxt1, b) {
			t.Errorf("contents mismatch %s, expected %s, received %s", exSubFile1, string(exSubTxt1), string(b))
		}
		err = fh.Close()
		if err != nil {
			t.Errorf("close %s: %v", exSubFile1, err)
		}

	})

	t.Run("openfile", func(t *testing.T) {
		// creating with OpenFile on existing directories should fail
		fh, err := rwfs.OpenFile(".", O_WRONLY, 0777)
		if err == nil {
			t.Errorf("creating \".\" with O_WRONLY did not fail")
			fh.Close()
		}
		fh, err = rwfs.OpenFile(exSubDir, O_WRONLY, 0777)
		if err == nil {
			t.Errorf("creating %s with O_WRONLY did not fail", exSubDir)
			fh.Close()
		}
		fh, err = rwfs.OpenFile(exSubDir, O_WRONLY|O_CREATE|O_EXCL, 0777)
		if err == nil {
			t.Errorf("creating %s with O_WRONLY|O_CREATE|O_EXCL did not fail", exSubDir)
			fh.Close()
		}

		// open dir with read/write
		fh, err = rwfs.OpenFile(exSubDir, O_RDWR|O_CREATE|O_EXCL, 0777)
		if err == nil {
			t.Errorf("creating %s with O_RDWR|O_CREATE|O_EXCL did not fail", exSubDir)
			fh.Close()
		}

		// read-writer from dir should fail
		_, err = rwfs.OpenFile(exSubDir, O_RDWR, 0777)
		if err == nil {
			t.Errorf("openfile %s with O_RDWR did not fail", exSubDir)
		}

		// read-only of dir should succeed, but Read and Write to that should fail
		fh, err = rwfs.OpenFile(exSubDir, O_RDONLY, 0777)
		if err != nil {
			t.Errorf("openfile %s with O_RDONLY: %v", exSubDir, err)
		}
		_, err = io.ReadAll(fh)
		if err == nil {
			t.Errorf("readall 2 %s did not fail", exSubDir)
		}
		_, err = fh.Write([]byte("hello world"))
		if err == nil {
			t.Errorf("write 2 %s did not fail", exSubDir)
		}
		fh.Close()

		// read file
		fh, err = rwfs.OpenFile(exSubFile2, O_RDONLY, 0666)
		if err != nil {
			t.Errorf("read-only %s: %v", exSubFile2, err)
		}
		b, err := io.ReadAll(fh)
		if err != nil {
			t.Errorf("readall %s: %v", exSubFile2, err)
		}
		if !bytes.Equal(b, exSubTxt2) {
			t.Errorf("readall %s: expected %s, received %s", exSubFile2, string(exSubTxt2), string(b))
		}
		fh.Close()

		// read-write file, read first
		fh, err = rwfs.OpenFile(exSubFile2, O_RDWR, 0666)
		if err != nil {
			t.Errorf("read-write %s: %v", exSubFile2, err)
		}
		b, err = io.ReadAll(fh)
		if err != nil {
			t.Errorf("readall %s: %v", exSubFile2, err)
		}
		if !bytes.Equal(b, exSubTxt2) {
			t.Errorf("contents after append, expect %s, received %s", string(exSubTxt2), string(b))
		}
		_, err = fh.Write([]byte("append string"))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		exB := append(exSubTxt2, []byte("append string")...)
		if !bytes.Equal(b, exB) {
			t.Errorf("contents after append, expect %s, received %s", string(exB), string(b))
		}

		// read-write file, w/o reading
		fh, err = rwfs.OpenFile(exSubFile2, O_RDWR, 0666)
		if err != nil {
			t.Errorf("read-write %s: %v", exSubFile2, err)
		}
		_, err = fh.Write([]byte("replace string"))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		copy(exB, []byte("replace string"))
		if !bytes.Equal(b, exB) {
			t.Errorf("contents after replace, expect %s, received %s", string(exB), string(b))
		}

		// read-write file+append
		fh, err = rwfs.OpenFile(exSubFile2, O_RDWR|O_APPEND, 0666)
		if err != nil {
			t.Errorf("read-write %s: %v", exSubFile2, err)
		}
		_, err = fh.Write([]byte("append again string"))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		exB = append(exB, []byte("append again string")...)
		if !bytes.Equal(b, exB) {
			t.Errorf("contents after append, expect %s, received %s", string(exB), string(b))
		}

		// read-write file+truncate
		fh, err = rwfs.OpenFile(exSubFile2, O_RDWR|O_TRUNC, 0666)
		if err != nil {
			t.Errorf("read-write %s: %v", exSubFile2, err)
		}
		_, err = fh.Write([]byte(exSubTxt2))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		if !bytes.Equal(b, exSubTxt2) {
			t.Errorf("contents after truncate, expect %s, received %s", string(exSubTxt2), string(b))
		}

		// write existing file with create+exclusive
		fh, err = rwfs.OpenFile(exSubFile2, O_WRONLY|O_CREATE|O_EXCL, 0666)
		if err == nil {
			t.Errorf("creating %s with O_WRONLY|O_CREATE|O_EXCL did not fail", exSubFile2)
			fh.Close()
		}

		// write existing file with create
		fh, err = rwfs.OpenFile(exSubFile2, O_WRONLY|O_CREATE, 0666)
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		_, err = fh.Write([]byte("replace"))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		exB = append([]byte{}, exSubTxt2...)
		copy(exB, []byte("replace"))
		if !bytes.Equal(b, exB) {
			t.Errorf("contents after create, expect %s, received %s", string(exB), string(b))
		}

		// write existing file with create+append
		fh, err = rwfs.OpenFile(exSubFile2, O_WRONLY|O_CREATE|O_APPEND, 0666)
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		_, err = fh.Write([]byte("append string"))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		exB = append(exB, []byte("append string")...)
		if !bytes.Equal(b, exB) {
			t.Errorf("contents after append, expect %s, received %s", string(exB), string(b))
		}

		// write existing file with create+truncate
		fh, err = rwfs.OpenFile(exSubFile2, O_WRONLY|O_CREATE|O_TRUNC, 0666)
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		_, err = fh.Write([]byte(exSubTxt2))
		if err != nil {
			t.Errorf("write %s: %v", exSubFile2, err)
		}
		fh.Close()
		b, err = fs.ReadFile(rwfs, exSubFile2)
		if err != nil {
			t.Errorf("readfile %s: %v", exSubFile2, err)
		}
		if !bytes.Equal(b, exSubTxt2) {
			t.Errorf("contents after truncate, expect %s, received %s", string(exSubTxt2), string(b))
		}

		// write new file exclusive w/o create
		fh, err = rwfs.OpenFile(exSubFile3, O_WRONLY, 0666)
		if err == nil {
			t.Errorf("write new file without create succeeded")
			fh.Close()
		}
		// write new file create+exclusive
		fh, err = rwfs.OpenFile(exSubFile3, O_WRONLY|O_CREATE|O_EXCL, 0666)
		if err != nil {
			t.Errorf("write new file with create failed: %v", err)
		}
		_, err = fh.Write(exSubTxt3)
		if err != nil {
			t.Errorf("write %s: %v", exSubFile3, err)
		}
		fh.Close()
	})

	// list root and subdir contents
	// open subdir
	t.Run("readdir", func(t *testing.T) {
		dl, err := fs.ReadDir(rwfs, ".")
		if err != nil {
			t.Errorf("readdir %s: %v", ".", err)
			return
		}
		if len(dl) != 3 {
			t.Errorf("too many entries in %s, expected %d, received %d", ".", 3, len(dl))
		}
		dm := map[string]fs.DirEntry{}
		for _, de := range dl {
			dm[de.Name()] = de
		}
		if de, ok := dm[exRootFile]; !ok {
			t.Errorf("missing file: %s", exRootFile)
		} else {
			if de.IsDir() {
				t.Errorf("file appears to be a directory: %s", exRootFile)
			}
			fi, err := de.Info()
			if err != nil {
				t.Errorf("fileinfo %s: %v", exRootFile, err)
			}
			if fi.Size() != int64(len(exRootTxt)) {
				t.Errorf("size mismatch %s, expected %d, received %d", exRootFile, len(exRootTxt), fi.Size())
			}
		}
		exDir := "nested"
		if de, ok := dm[exDir]; !ok {
			t.Errorf("missing dir: %s", exDir)
		} else {
			if !de.IsDir() {
				t.Errorf("dir failed IsDir: %s", exDir)
			}
			_, err := de.Info()
			if err != nil {
				t.Errorf("fileinfo %s: %v", exDir, err)
			}
		}
		if de, ok := dm[exSubDir]; !ok {
			t.Errorf("missing dir: %s", exSubDir)
		} else {
			if !de.IsDir() {
				t.Errorf("dir failed IsDir: %s", exSubDir)
			}
			_, err := de.Info()
			if err != nil {
				t.Errorf("fileinfo %s: %v", exSubDir, err)
			}
		}

		dl, err = fs.ReadDir(rwfs, exNestedDir)
		if err != nil {
			t.Errorf("readdir %s: %v", exNestedDir, err)
			return
		}
		if len(dl) != 2 {
			t.Errorf("too many entries in %s, expected %d, received %d", exNestedDir, 2, len(dl))
		}
		dm = map[string]fs.DirEntry{}
		for _, de := range dl {
			dm[de.Name()] = de
		}
		exFile := "file"
		if de, ok := dm[exFile]; !ok {
			t.Errorf("missing file: %s", exFile)
		} else {
			if de.IsDir() {
				t.Errorf("file appears to be a directory: %s", exFile)
			}
			fi, err := de.Info()
			if err != nil {
				t.Errorf("fileinfo %s: %v", exFile, err)
			}
			if fi.Size() != int64(len(exNestedTxt2)) {
				t.Errorf("size mismatch %s, expected %d, received %d", exFile, len(exNestedTxt2), fi.Size())
			}
		}
		exDir = "tree"
		if de, ok := dm[exDir]; !ok {
			t.Errorf("missing dir: %s", exDir)
		} else {
			if !de.IsDir() {
				t.Errorf("dir failed IsDir: %s", exDir)
			}
			_, err := de.Info()
			if err != nil {
				t.Errorf("fileinfo %s: %v", exDir, err)
			}
		}
	})

	t.Run("copy", func(t *testing.T) {
		err := CopyRecursive(rwfs, "nested", rwfs, "copy")
		if err != nil {
			t.Errorf("CopyRecursive to copy dir failed: %v", err)
		}
		fi, err := Stat(rwfs, "copy/dir/file")
		if err != nil {
			t.Errorf("file not copied: %v", err)
		}
		if fi.IsDir() {
			t.Errorf("file appears to be a directory")
		}
		curMemFS := MemNew()
		err = CopyRecursive(rwfs, ".", curMemFS, ".")
		if err != nil {
			t.Errorf("CopyRecursive to memfs failed: %v", err)
		}
		fi, err = Stat(rwfs, exNestedFile)
		if err != nil {
			t.Errorf("file not copied: %v", err)
		}
		if fi.IsDir() {
			t.Errorf("file appears to be a directory")
		}
	})

	t.Run("rename", func(t *testing.T) {
		err := rwfs.Rename("nested", "renamed")
		if err != nil {
			t.Errorf("failed renaming: %v", err)
		}
		_, err = Stat(rwfs, "nested")
		if err == nil {
			t.Errorf("nested exists after rename")
		}
		fi, err := Stat(rwfs, "renamed")
		if err != nil {
			t.Errorf("renamed folder not found: %v", err)
		}
		if !fi.IsDir() {
			t.Errorf("renamed folder is not a directory")
		}
		err = rwfs.Rename("renamed", "nested")
		if err != nil {
			t.Errorf("failed renaming back: %v", err)
		}
		fi, err = Stat(rwfs, "nested")
		if err != nil {
			t.Errorf("nested not recreated after rename: %v", err)
		}
		if !fi.IsDir() {
			t.Errorf("nested is not a directory after rename")
		}
		_, err = Stat(rwfs, "renamed")
		if err == nil {
			t.Errorf("renamed exists after rename")
		}
	})

	t.Run("remove", func(t *testing.T) {
		err := rwfs.Remove(".")
		if err == nil {
			t.Errorf("did not fail deleting \".\"")
		}
		err = rwfs.Remove(exNestedDir)
		if err == nil {
			t.Errorf("did not fail deleting %s before deleting children", exNestedDir)
		}
		err = rwfs.Remove(exNestedFile)
		if err != nil {
			t.Errorf("failed deleting %s: %v", exNestedFile, err)
		}
		err = rwfs.Remove(exNestedChildDir)
		if err != nil {
			t.Errorf("failed deleting %s: %v", exNestedChildDir, err)
		}
		err = rwfs.Remove(exNestedDir)
		if err != nil {
			t.Errorf("failed deleting %s: %v", exNestedDir, err)
		}
		fh, err := rwfs.Open(exNestedDir)
		if err == nil {
			t.Errorf("open succeeded after deleting %s", exNestedDir)
			fh.Close()
		}
	})

	t.Run("CreateTemp", func(t *testing.T) {
		err := MkdirAll(rwfs, "tempdir", 0700)
		if err != nil {
			t.Errorf("failed creating tempdir: %v", err)
		}
		fh, err := CreateTemp(rwfs, "tempdir", "tempfile")
		if err != nil {
			t.Errorf("failed creating temp file: %v", err)
			return
		}
		defer fh.Close()
		fhStat, err := fh.Stat()
		if err != nil {
			t.Errorf("stat failed: %v", err)
		}
		tmpName := filepath.Join("tempdir", fhStat.Name())
		defer rwfs.Remove(tmpName)
		if !strings.HasPrefix(fhStat.Name(), "tempfile") {
			t.Errorf("filename prefix mismatch, expected tempfile, received name %s", fhStat.Name())
		}
	})
}
