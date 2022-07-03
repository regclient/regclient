package mod

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
)

// WithBuildArgRm removes a build arg from the config history
func WithBuildArgRm(arg string, value *regexp.Regexp) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}

// WithConfigTimestampFromLabel sets the max timestamp in the config to match a label value
func WithConfigTimestampFromLabel(label string) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}

// WithConfigTimestampMax sets the max timestamp on any config objects
func WithConfigTimestampMax(t time.Time) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
			changed := false
			oc := doc.oc.GetConfig()
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
	}
}

// WithExposeAdd defines an exposed port in the image config
func WithExposeAdd(port string) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}

// WithExposeRm deletes an exposed from the image config
func WithExposeRm(port string) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}

// WithLabel sets or deletes a label from the image config
func WithLabel(name, value string) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}

// WithVolumeAdd defines a volume in the image config
func WithVolumeAdd(volume string) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}

// WithVolumeRm deletes a volume from the image config
func WithVolumeRm(volume string) Opts {
	return func(dc *dagConfig) {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, r ref.Ref, doc *dagOCIConfig) error {
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
	}
}
