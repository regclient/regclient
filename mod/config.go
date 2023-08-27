package mod

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

// WithBuildArgRm removes a build arg from the config history
func WithBuildArgRm(arg string, value *regexp.Regexp) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
			argexp := regexp.MustCompile(fmt.Sprintf(`(?s)^ARG %s(=.*|)$`,
				regexp.QuoteMeta(arg)))
			runexp := regexp.MustCompile(fmt.Sprintf(`(?s)^RUN \|([0-9]+) (.*)%s=%s(.*)$`,
				regexp.QuoteMeta(arg), value.String()))
			for i := len(oc.History) - 1; i >= 0; i-- {
				if argexp.MatchString(oc.History[i].CreatedBy) && oc.History[i].EmptyLayer {
					// delete empty build arg history entry
					oc.History = append(oc.History[:i], oc.History[i+1:]...)
					changed = true
				} else if match := runexp.FindStringSubmatch(oc.History[i].CreatedBy); len(match) == 4 {
					// delete arg from run steps
					count, err := strconv.Atoi(match[1])
					if err != nil {
						return fmt.Errorf("failed parsing history \"%s\": %w", oc.History[i].CreatedBy, err)
					}
					if count == 1 && len(match[2]) == 0 {
						oc.History[i].CreatedBy = fmt.Sprintf("RUN %s", match[3])
					} else {
						oc.History[i].CreatedBy = fmt.Sprintf("RUN |%d %s%s", count-1, match[2], match[3])
					}
					changed = true
				}
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.modified = true
			}
			return nil
		})
		return nil
	}
}

// WithConfigTimestamp sets the timestamp on the config entries based on options
func WithConfigTimestamp(optTime OptTime) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		if optTime.Set.IsZero() && optTime.FromLabel == "" {
			return fmt.Errorf("WithConfigTimestamp requires a time to set")
		}
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			oc := doc.oc.GetConfig()
			// lookup start time from label
			if optTime.FromLabel != "" {
				tl, ok := oc.Config.Labels[optTime.FromLabel]
				if !ok {
					return fmt.Errorf("label not found: %s", optTime.FromLabel)
				}
				tNew, err := time.Parse(time.RFC3339, tl)
				if err != nil {
					// TODO: add fallbacks
					return fmt.Errorf("could not parse time %s from %s: %w", tl, optTime.FromLabel, err)
				}
				if !optTime.Set.IsZero() && !optTime.Set.Equal(tNew) {
					return fmt.Errorf("conflicting time labels found %s and %s", optTime.Set.String(), tNew.String())
				}
				optTime.Set = tNew
			}
			startHistory := 0
			// offset startHistory by base layer count
			if optTime.BaseLayers > 0 {
				layersCount := 0
				for i, history := range oc.History {
					if !history.EmptyLayer {
						layersCount++
					}
					if layersCount == optTime.BaseLayers {
						startHistory = i + 1
						break
					}
				}
				if startHistory == 0 {
					startHistory = len(oc.History)
				}
			}
			// offset startHistory from base image history
			if !optTime.BaseRef.IsZero() {
				var d types.Descriptor
				for {
					mOpts := []regclient.ManifestOpts{}
					if d.Digest != "" {
						mOpts = append(mOpts, regclient.WithManifestDesc(d))
					}
					m, err := rc.ManifestGet(c, optTime.BaseRef, mOpts...)
					if err != nil {
						return fmt.Errorf("unable to get base image: %w", err)
					}
					if mi, ok := m.(manifest.Imager); ok {
						cd, err := mi.GetConfig()
						if err != nil {
							return fmt.Errorf("unable to get base image config descriptor: %w", err)
						}
						d = cd
						break
					} else if _, ok := m.(manifest.Indexer); ok {
						pd, err := manifest.GetPlatformDesc(m, &oc.Platform)
						if err != nil {
							return fmt.Errorf("unable to get base image platform %s: %w", oc.Platform.String(), err)
						}
						d = *pd
					} else {
						return fmt.Errorf("unsupported base image manifest")
					}
				}
				baseConfig, err := rc.BlobGetOCIConfig(c, optTime.BaseRef, d)
				if err != nil {
					return fmt.Errorf("failed to get base image config: %w", err)
				}
				// exclude matching history lines from base image
				for i, history := range baseConfig.GetConfig().History {
					if len(oc.History) <= i ||
						oc.History[i].Author != history.Author ||
						oc.History[i].Comment != history.Comment ||
						!oc.History[i].Created.Equal(*history.Created) ||
						oc.History[i].CreatedBy != history.CreatedBy ||
						oc.History[i].EmptyLayer != history.EmptyLayer {
						break
					}
					startHistory = i + 1
				}
			}
			var changed, cCur bool
			// adjust created time on created and history fields
			if oc.Created != nil {
				*oc.Created, changed = timeModOpt(*oc.Created, optTime)
			}
			for i := startHistory; i < len(oc.History); i++ {
				*oc.History[i].Created, cCur = timeModOpt(*oc.History[i].Created, optTime)
				changed = changed || cCur
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.newDesc = doc.oc.GetDescriptor()
				doc.modified = true
			}
			return nil
		})
		return nil
	}
}

// WithConfigTimestampFromLabel sets the max timestamp in the config to match a label value
//
// Deprecated: replace with WithConfigTimestamp
func WithConfigTimestampFromLabel(label string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			var err error
			changed := false
			oc := doc.oc.GetConfig()
			tl, ok := oc.Config.Labels[label]
			if !ok {
				return fmt.Errorf("label not found: %s", label)
			}
			t, err := time.Parse(time.RFC3339, tl)
			if err != nil {
				// TODO: add fallbacks
				return fmt.Errorf("could not parse time %s from %s: %w", tl, label, err)
			}
			if oc.Created != nil && t.Before(*oc.Created) {
				*oc.Created = t
				changed = true
			}
			if oc.History != nil {
				for i, h := range oc.History {
					if h.Created != nil && t.Before(*h.Created) {
						*oc.History[i].Created = t
						changed = true
					}
				}
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.newDesc = doc.oc.GetDescriptor()
				doc.modified = true
			}
			return nil
		})
		return nil
	}
}

// WithConfigTimestampMax sets the max timestamp on any config objects
//
// Deprecated: replace with WithConfigTimestamp
func WithConfigTimestampMax(t time.Time) Opts {
	return WithConfigTimestamp(OptTime{
		Set:   t,
		After: t,
	})
}

// WithExposeAdd defines an exposed port in the image config
func WithExposeAdd(port string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
			if oc.Config.ExposedPorts == nil {
				oc.Config.ExposedPorts = map[string]struct{}{}
			}
			if _, ok := oc.Config.ExposedPorts[port]; !ok {
				changed = true
				oc.Config.ExposedPorts[port] = struct{}{}
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.modified = true
				doc.newDesc = doc.oc.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}

// WithExposeRm deletes an exposed from the image config
func WithExposeRm(port string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
			if oc.Config.ExposedPorts == nil {
				return nil
			}
			if _, ok := oc.Config.ExposedPorts[port]; ok {
				changed = true
				delete(oc.Config.ExposedPorts, port)
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.modified = true
				doc.newDesc = doc.oc.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}

// WithLabel sets or deletes a label from the image config
func WithLabel(name, value string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
			if oc.Config.Labels == nil {
				oc.Config.Labels = map[string]string{}
			}
			cur, ok := oc.Config.Labels[name]
			if value == "" && ok {
				delete(oc.Config.Labels, name)
				changed = true
			} else if value != "" && value != cur {
				oc.Config.Labels[name] = value
				changed = true
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.modified = true
				doc.newDesc = doc.oc.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}

// WithVolumeAdd defines a volume in the image config
func WithVolumeAdd(volume string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
			if oc.Config.Volumes == nil {
				oc.Config.Volumes = map[string]struct{}{}
			}
			if _, ok := oc.Config.Volumes[volume]; !ok {
				changed = true
				oc.Config.Volumes[volume] = struct{}{}
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.modified = true
				doc.newDesc = doc.oc.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}

// WithVolumeRm deletes a volume from the image config
func WithVolumeRm(volume string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
			if oc.Config.Volumes == nil {
				return nil
			}
			if _, ok := oc.Config.Volumes[volume]; ok {
				changed = true
				delete(oc.Config.Volumes, volume)
			}
			if changed {
				doc.oc.SetConfig(oc)
				doc.modified = true
				doc.newDesc = doc.oc.GetDescriptor()
			}
			return nil
		})
		return nil
	}
}
