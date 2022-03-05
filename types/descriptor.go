package types

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/units"
	"github.com/regclient/regclient/types/platform"
)

// Descriptor is used in manifests to refer to content by media type, size, and digest.
type Descriptor struct {
	// MediaType describe the type of the content.
	MediaType string `json:"mediaType,omitempty"`

	// Size in bytes of content.
	Size int64 `json:"size,omitempty"`

	// Digest uniquely identifies the content.
	Digest digest.Digest `json:"digest,omitempty"`

	// URLs contains the source URLs of this content.
	URLs []string `json:"urls,omitempty"`

	// Annotations contains arbitrary metadata relating to the targeted content.
	Annotations map[string]string `json:"annotations,omitempty"`

	// Platform describes the platform which the image in the manifest runs on.
	// This should only be used when referring to a manifest.
	Platform *platform.Platform `json:"platform,omitempty"`
}

func (d Descriptor) MarshalPrettyTW(tw *tabwriter.Writer, prefix string) error {
	fmt.Fprintf(tw, "%sDigest:\t%s\n", prefix, string(d.Digest))
	fmt.Fprintf(tw, "%sMediaType:\t%s\n", prefix, d.MediaType)
	switch d.MediaType {
	case MediaTypeDocker1Manifest, MediaTypeDocker1ManifestSigned,
		MediaTypeDocker2Manifest, MediaTypeDocker2ManifestList,
		MediaTypeOCI1Manifest, MediaTypeOCI1ManifestList:
		// skip printing size for descriptors to manifests
	default:
		if d.Size > 100000 {
			fmt.Fprintf(tw, "%sSize:\t%s\n", prefix, units.HumanSize(float64(d.Size)))
		} else {
			fmt.Fprintf(tw, "%sSize:\t%dB\n", prefix, d.Size)
		}
	}
	if p := d.Platform; p != nil && p.OS != "" {
		fmt.Fprintf(tw, "%sPlatform:\t%s\n", prefix, p.String())
		if p.OSVersion != "" {
			fmt.Fprintf(tw, "%sOSVersion:\t%s\n", prefix, p.OSVersion)
		}
		if len(p.OSFeatures) > 0 {
			fmt.Fprintf(tw, "%sOSFeatures:\t%s\n", prefix, strings.Join(p.OSFeatures, ", "))
		}
	}
	if len(d.URLs) > 0 {
		fmt.Fprintf(tw, "%sURLs:\t%s\n", prefix, strings.Join(d.URLs, ", "))
	}
	if d.Annotations != nil {
		fmt.Fprintf(tw, "%sAnnotations:\t\n", prefix)
		for k, v := range d.Annotations {
			fmt.Fprintf(tw, "%s  %s:\t%s\n", prefix, k, v)
		}
	}
	return nil
}
