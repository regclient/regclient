package mod

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/blob"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

// WithBuildArgRm removes a build arg from the config history.
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
					oc.History = slices.Delete(oc.History, i, i+1)
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

// WithConfigCmd sets the command in the config.
// For running a shell command, the `cmd` value should be `[]string{"/bin/sh", "-c", command}`.
func WithConfigCmd(cmd []string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			oc := doc.oc.GetConfig()
			if slices.Equal(cmd, oc.Config.Cmd) {
				return nil
			}
			oc.Config.Cmd = cmd
			doc.oc.SetConfig(oc)
			doc.modified = true
			return nil
		})
		return nil
	}
}

// WithConfigDigestAlgo changes the digest algorithm.
func WithConfigDigestAlgo(algo digest.Algorithm) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		if !algo.Available() {
			return fmt.Errorf("digest algorithm is not available: %s", string(algo))
		}
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			desc := doc.oc.GetDescriptor()
			if doc.newDesc.MediaType != "" {
				desc = doc.newDesc
			}
			if desc.DigestAlgo() == algo {
				return nil
			}
			if !algo.Available() {
				return fmt.Errorf("unavailable digest algorithm: %s", string(algo))
			}
			body, err := doc.oc.RawBody()
			if err != nil {
				return fmt.Errorf("failed to get config body: %w", err)
			}
			desc.Digest = algo.FromBytes(body)
			doc.oc = blob.NewOCIConfig(
				blob.WithDesc(desc),
				blob.WithRawBody(body),
			)
			doc.modified = true
			return nil
		})
		return nil
	}
}

// WithConfigEntrypoint sets the entrypoint in the config.
// For running a shell command, the `entrypoint` value should be `[]string{"/bin/sh", "-c", command}`.
func WithConfigEntrypoint(entrypoint []string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			oc := doc.oc.GetConfig()
			if slices.Equal(entrypoint, oc.Config.Entrypoint) {
				return nil
			}
			oc.Config.Entrypoint = entrypoint
			doc.oc.SetConfig(oc)
			doc.modified = true
			return nil
		})
		return nil
	}
}

// WithConfigPlatform sets the platform in the config.
func WithConfigPlatform(p platform.Platform) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(ctx context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			oc := doc.oc.GetConfig()
			if platform.Match(oc.Platform, p) {
				return nil
			}
			oc.Platform = p
			doc.oc.SetConfig(oc)
			doc.modified = true
			return nil
		})
		return nil
	}
}

// WithConfigTimestamp sets the timestamp on the config entries based on options.
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
				baseConfig, err := rc.ImageConfig(c, optTime.BaseRef, regclient.ImageWithPlatform(oc.Platform.String()))
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

// WithConfigTimestampFromLabel sets the max timestamp in the config to match a label value.
//
// Deprecated: replace with [WithConfigTimestamp].
//
//go:fix inline
func WithConfigTimestampFromLabel(label string) Opts {
	return WithConfigTimestamp(OptTime{FromLabel: label})
}

// WithConfigTimestampMax sets the max timestamp on any config objects.
//
// Deprecated: replace with [WithConfigTimestamp].
//
//go:fix inline
func WithConfigTimestampMax(t time.Time) Opts {
	return WithConfigTimestamp(OptTime{
		Set:   t,
		After: t,
	})
}

// WithEnv sets or deletes an environment variable from the image config.
func WithEnv(name, value string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		// extract the list for platforms to update from the name
		name = strings.TrimSpace(name)
		platforms := []platform.Platform{}
		if name[0] == '[' && strings.Index(name, "]") > 0 {
			end := strings.Index(name, "]")
			for entry := range strings.SplitSeq(name[1:end], ",") {
				entry = strings.TrimSpace(entry)
				if entry == "*" {
					continue
				}
				p, err := platform.Parse(entry)
				if err != nil {
					return fmt.Errorf("failed to parse env platform %s: %w", entry, err)
				}
				platforms = append(platforms, p)
			}
			name = name[end+1:]
		}
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			// if platforms are listed, skip non-matching platforms
			if len(platforms) > 0 {
				p := doc.oc.GetConfig().Platform
				found := false
				for _, pe := range platforms {
					if platform.Match(p, pe) {
						found = true
						break
					}
				}
				if !found {
					return nil
				}
			}
			changed := false
			found := false
			oc := doc.oc.GetConfig()
			for i, kv := range oc.Config.Env {
				kvSplit := strings.SplitN(kv, "=", 2)
				if kvSplit[0] != name {
					continue
				}
				found = true
				if value == "" {
					// delete an entry
					oc.Config.Env = slices.Delete(oc.Config.Env, i, i+1)
					changed = true
				} else if len(kvSplit) < 2 || value != kvSplit[1] {
					// change an entry
					oc.Config.Env[i] = name + "=" + value
					changed = true
				}
				break
			}
			if !found && value != "" {
				// add a new entry
				oc.Config.Env = append(oc.Config.Env, name+"="+value)
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

// WithExposeAdd defines an exposed port in the image config.
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

// WithExposeRm deletes an exposed from the image config.
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

// WithLabel sets or deletes a label from the image config.
func WithLabel(name, value string) Opts {
	return func(dc *dagConfig, dm *dagManifest) error {
		// extract the list for platforms to update from the name
		name = strings.TrimSpace(name)
		platforms := []platform.Platform{}
		if name[0] == '[' && strings.Index(name, "]") > 0 {
			end := strings.Index(name, "]")
			for entry := range strings.SplitSeq(name[1:end], ",") {
				entry = strings.TrimSpace(entry)
				if entry == "*" {
					continue
				}
				p, err := platform.Parse(entry)
				if err != nil {
					return fmt.Errorf("failed to parse label platform %s: %w", entry, err)
				}
				platforms = append(platforms, p)
			}
			name = name[end+1:]
		}
		dc.stepsOCIConfig = append(dc.stepsOCIConfig, func(c context.Context, rc *regclient.RegClient, rSrc, rTgt ref.Ref, doc *dagOCIConfig) error {
			// if platforms are listed, skip non-matching platforms
			if len(platforms) > 0 {
				p := doc.oc.GetConfig().Platform
				found := false
				for _, pe := range platforms {
					if platform.Match(p, pe) {
						found = true
						break
					}
				}
				if !found {
					return nil
				}
			}
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

// WithVolumeAdd defines a volume in the image config.
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

// WithVolumeRm deletes a volume from the image config.
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
