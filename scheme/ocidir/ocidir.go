package ocidir

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"path"

	ociSpecs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

const (
	aRefName = "org.opencontainers.image.ref.name"
)

type OCIDir struct {
	fs  rwfs.RWFS
	log *logrus.Logger
	gc  bool
}

type Config struct {
	fs  rwfs.RWFS
	gc  bool
	log *logrus.Logger
}

type Opts func(*Config)

func New(opts ...Opts) *OCIDir {
	conf := Config{
		log: &logrus.Logger{Out: ioutil.Discard},
	}
	for _, opt := range opts {
		opt(&conf)
	}
	return &OCIDir{
		fs:  conf.fs,
		log: conf.log,
		gc:  conf.gc,
	}
}
func WithFS(fs rwfs.RWFS) Opts {
	return func(c *Config) {
		c.fs = fs
	}
}
func WithGC(gc bool) Opts {
	return func(c *Config) {
		c.gc = gc
	}
}
func WithLog(log *logrus.Logger) Opts {
	return func(c *Config) {
		c.log = log
	}
}

func (o *OCIDir) Info() scheme.Info {
	return scheme.Info{ManifestPushFirst: true}
}

func (o *OCIDir) readIndex(r ref.Ref) (ociv1.Index, error) {
	// validate dir
	index := ociv1.Index{}
	err := o.valid(r.Path)
	if err != nil {
		return index, err
	}
	indexFile := path.Join(r.Path, "index.json")
	fh, err := o.fs.Open(indexFile)
	if err != nil {
		return index, fmt.Errorf("%s cannot be open: %w", indexFile, err)
	}
	defer fh.Close()
	ib, err := io.ReadAll(fh)
	if err != nil {
		return index, fmt.Errorf("%s cannot be read: %w", indexFile, err)
	}
	err = json.Unmarshal(ib, &index)
	if err != nil {
		return index, fmt.Errorf("%s cannot be parsed: %w", indexFile, err)
	}
	return index, nil
}

func (o *OCIDir) writeIndex(r ref.Ref, i ociv1.Index) error {
	// create/replace oci-layout file
	layout := ociv1.ImageLayout{
		Version: "1.0.0",
	}
	lb, err := json.Marshal(layout)
	lfh, err := o.fs.Create(path.Join(r.Path, ociv1.ImageLayoutFile))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", ociv1.ImageLayoutFile, err)
	}
	defer lfh.Close()
	_, err = lfh.Write(lb)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", ociv1.ImageLayoutFile, err)
	}
	// create/replace index.json file
	indexFile := path.Join(r.Path, "index.json")
	fh, err := o.fs.Create(indexFile)
	if err != nil {
		return fmt.Errorf("%s cannot be created: %w", indexFile, err)
	}
	defer fh.Close()
	b, err := json.Marshal(i)
	if err != nil {
		return fmt.Errorf("cannot marshal index: %w", err)
	}
	_, err = fh.Write(b)
	if err != nil {
		return fmt.Errorf("cannot write index: %w", err)
	}
	return nil
}

// func valid (dir) (error) // check for `oci-layout` file and `index.json` for read
func (o *OCIDir) valid(dir string) error {
	layout := ociv1.ImageLayout{}
	reqVer := "1.0.0"
	if !fs.ValidPath(dir) {
		return fmt.Errorf("%w: %s is not a valid path", types.ErrParsingFailed, dir)
	}
	fh, err := o.fs.Open(path.Join(dir, ociv1.ImageLayoutFile))
	if err != nil {
		return fmt.Errorf("%s cannot be open: %w", ociv1.ImageLayoutFile, err)
	}
	defer fh.Close()
	lb, err := io.ReadAll(fh)
	if err != nil {
		return fmt.Errorf("%s cannot be read: %w", ociv1.ImageLayoutFile, err)
	}
	err = json.Unmarshal(lb, &layout)
	if err != nil {
		return fmt.Errorf("%s cannot be parsed: %w", ociv1.ImageLayoutFile, err)
	}
	if layout.Version != reqVer {
		return fmt.Errorf("unsupported oci layout version, expected %s, received %s", reqVer, layout.Version)
	}
	return nil
}

func indexCreate() ociv1.Index {
	i := ociv1.Index{
		Versioned: ociSpecs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:   types.MediaTypeOCI1ManifestList,
		Manifests:   []ociv1.Descriptor{},
		Annotations: map[string]string{},
	}
	return i
}

func indexRefLookup(index ociv1.Index, r ref.Ref) (int, error) {
	// make 2 passes, first for the tag, and second for the digest without a tag
	// one digest could be tagged multiple times in the index
	if r.Tag != "" {
		for i, im := range index.Manifests {
			if name, ok := im.Annotations[aRefName]; ok && name == r.Tag {
				return i, nil
			}
		}
	}
	if r.Digest != "" {
		for i, im := range index.Manifests {
			if _, ok := im.Annotations[aRefName]; !ok && im.Digest.String() == r.Digest {
				return i, nil
			}
		}
	}
	return 0, types.ErrNotFound
}

// func (*OCIDir) readDesc (ref, digest) (readcloser, error)
// func (*OCIDir) createDesc (ref, digest) (writecloser, error)
// func (*OCIDir) writeDesc (ref, digest, []byte) (error)
