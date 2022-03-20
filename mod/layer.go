package mod

import (
	"archive/tar"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
)

// WithLayerRmCreatedBy deletes a layer based on a regex of the created by field
// in the config history for that layer
func WithLayerRmCreatedBy(re regexp.Regexp) Opts {
	return func(dc *dagConfig) {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, r ref.Ref, dm *dagManifest) error {
			if dm.m.IsList() || dm.config.oc == nil {
				return nil
			}
			if dm.layers == nil || len(dm.layers) == 0 {
				return fmt.Errorf("no layers found")
			}
			delLayers := []int{}
			oc := dm.config.oc.GetConfig()
			i := 0
			for _, ch := range oc.History {
				if ch.EmptyLayer {
					continue
				}
				if re.Match([]byte(ch.CreatedBy)) {
					delLayers = append(delLayers, i)
				}
				i++
			}
			if len(delLayers) == 0 {
				return fmt.Errorf("no layers match expression: %s", re.String())
			}
			curLayer := 0
			curOrigLayer := 0
			for _, i := range delLayers {
				for {
					if len(dm.layers) <= curLayer {
						return fmt.Errorf("layers missing")
					}
					if dm.layers[curLayer].mod == added {
						curLayer++
						continue
					}
					if curOrigLayer == i {
						dm.layers[curLayer].mod = deleted
						break
					}
					curLayer++
					curOrigLayer++
				}
			}
			return nil
		})
	}
}

// WithLayerRmIndex deletes a layer by index. The index starts at 0.
func WithLayerRmIndex(index int) Opts {
	return func(dc *dagConfig) {
		dc.stepsManifest = append(dc.stepsManifest, func(c context.Context, rc *regclient.RegClient, r ref.Ref, dm *dagManifest) error {
			if !dm.top || dm.m.IsList() || dm.config.oc == nil {
				return fmt.Errorf("remove layer by index requires v2 image manifest")
			}
			if dm.layers == nil || len(dm.layers) == 0 {
				return fmt.Errorf("no layers found")
			}
			curLayer := 0
			curOrigLayer := 0
			for {
				if len(dm.layers) <= curLayer {
					return fmt.Errorf("layer not found")
				}
				if dm.layers[curLayer].mod == added {
					curLayer++
					continue
				}
				if curOrigLayer == index {
					dm.layers[curLayer].mod = deleted
					break
				}
				curLayer++
				curOrigLayer++
			}
			return nil
		})
	}
}

// WithLayerStripFile removes a file from within the layer tar
func WithLayerStripFile(file string) Opts {
	file = strings.Trim(file, "/")
	fileRE := regexp.MustCompile("^/?" + regexp.QuoteMeta(file) + "(/.*)?$")
	return func(dc *dagConfig) {
		dc.stepsLayerFile = append(dc.stepsLayerFile, func(c context.Context, rc *regclient.RegClient, r ref.Ref, dl *dagLayer, th *tar.Header, tr *tar.Reader) (*tar.Header, *tar.Reader, changes, error) {
			if fileRE.Match([]byte(th.Name)) {
				return th, tr, deleted, nil
			}
			return th, tr, unchanged, nil
		})
	}
}

// WithLayerTimestampFromLabel sets the max layer timestamp based on a label in the image
func WithLayerTimestampFromLabel(label string) Opts {
	t := time.Time{}
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
			oc := doc.oc.GetConfig()
			tl, ok := oc.Config.Labels[label]
			if !ok {
				return fmt.Errorf("label not found: %s", label)
			}
			tNew, err := time.Parse(time.RFC3339, tl)
			if err != nil {
				// TODO: add fallbacks
				return fmt.Errorf("could not parse time %s from %s: %w", tl, label, err)
			}
			if !t.IsZero() && !t.Equal(tNew) {
				return fmt.Errorf("conflicting time labels found %s and %s", t.String(), tNew.String())
			}
			t = tNew
			return nil
		})
		dc.stepsLayerFile = append(dc.stepsLayerFile,
			func(c context.Context, rc *regclient.RegClient, r ref.Ref, dl *dagLayer, th *tar.Header, tr *tar.Reader) (*tar.Header, *tar.Reader, changes, error) {
				if t.IsZero() {
					return nil, nil, unchanged, fmt.Errorf("timestamp not available")
				}
				changed := false
				if th == nil || tr == nil {
					return nil, nil, unchanged, fmt.Errorf("missing header or reader")
				}
				if t.Before(th.AccessTime) {
					th.AccessTime = t
					changed = true
				}
				if t.Before(th.ChangeTime) {
					th.ChangeTime = t
					changed = true
				}
				if t.Before(th.ModTime) {
					th.ModTime = t
					changed = true
				}
				if changed {
					return th, tr, replaced, nil
				}
				return th, tr, unchanged, nil
			},
		)
	}
}

// WithLayerTimestampMax ensures no file timestamps are after specified time
func WithLayerTimestampMax(t time.Time) Opts {
	return func(dc *dagConfig) {
		dc.stepsLayerFile = append(dc.stepsLayerFile,
			func(c context.Context, rc *regclient.RegClient, r ref.Ref, dl *dagLayer, th *tar.Header, tr *tar.Reader) (*tar.Header, *tar.Reader, changes, error) {
				changed := false
				if th == nil || tr == nil {
					return nil, nil, unchanged, fmt.Errorf("missing header or reader")
				}
				if t.Before(th.AccessTime) {
					th.AccessTime = t
					changed = true
				}
				if t.Before(th.ChangeTime) {
					th.ChangeTime = t
					changed = true
				}
				if t.Before(th.ModTime) {
					th.ModTime = t
					changed = true
				}
				if changed {
					return th, tr, replaced, nil
				}
				return th, tr, unchanged, nil
			},
		)
	}
}
