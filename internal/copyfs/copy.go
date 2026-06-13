// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package copyfs is used internally to recursively copy filesystem content.
package copyfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Copy copies the content of src to dst.
func Copy(dest, src string) error {
	dest = filepath.Clean(dest)
	src = filepath.Clean(src)
	return filepath.Walk(src, func(srcCur string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		destCur := filepath.Join(dest, strings.TrimPrefix(srcCur, src))
		// handle directory
		if fi.IsDir() {
			err = os.MkdirAll(destCur, fi.Mode())
			return err
		}
		// handle links and other files
		if !fi.Mode().IsRegular() {
			switch fi.Mode().Type() & os.ModeType {
			case os.ModeSymlink:
				link, err := os.Readlink(srcCur)
				if err != nil {
					return err
				}
				//#nosec G122 copy is only used for internal (test) code.
				return os.Symlink(link, destCur)
			default:
				return fmt.Errorf("unsupported file to copy: %s, type = %d", srcCur, fi.Mode().Type())
			}
		}
		// copy file
		//#nosec G122 G304 copy is only used for internal (test) code.
		fhSrc, err := os.Open(srcCur)
		if err != nil {
			return err
		}
		defer fhSrc.Close()
		//#nosec G304 copy is only used for internal (test) code.
		fhDest, err := os.Create(destCur)
		if err != nil {
			return err
		}
		defer fhDest.Close()
		err = fhDest.Chmod(fi.Mode())
		if err != nil {
			return err
		}
		_, err = io.Copy(fhDest, fhSrc)
		return err
	})
}
