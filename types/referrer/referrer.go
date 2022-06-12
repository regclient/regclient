// Package referrer is used for responses to the referrers to a manifest
package referrer

import (
	"bytes"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

type ReferrerList struct {
	Ref         ref.Ref            `json:"ref"`                   // reference queried
	Descriptors []types.Descriptor `json:"descriptors"`           // descriptors found in Index
	Annotations map[string]string  `json:"annotations,omitempty"` // annotations extracted from Index
	Manifest    manifest.Manifest  `json:"-"`                     // returned OCI Index
	Tags        []string           `json:"-"`                     // tags matched when fetching referrers
}

// MarshalPretty is used for printPretty template formatting
func (rl ReferrerList) MarshalPretty() ([]byte, error) {
	buf := &bytes.Buffer{}
	tw := tabwriter.NewWriter(buf, 0, 0, 1, ' ', 0)
	if rl.Ref.Reference != "" {
		fmt.Fprintf(tw, "Refers:\t%s\n", rl.Ref.Reference)
	}
	rRef := rl.Ref
	rRef.Tag = ""
	fmt.Fprintf(tw, "\t\n")
	fmt.Fprintf(tw, "Referrers:\t\n")
	for _, d := range rl.Descriptors {
		fmt.Fprintf(tw, "\t\n")
		if rRef.Reference != "" {
			rRef.Digest = d.Digest.String()
			fmt.Fprintf(tw, "  Name:\t%s\n", rRef.CommonName())
		}
		err := d.MarshalPrettyTW(tw, "  ")
		if err != nil {
			return []byte{}, err
		}
	}
	if rl.Annotations != nil && len(rl.Annotations) > 0 {
		fmt.Fprintf(tw, "Annotations:\t\n")
		keys := make([]string, 0, len(rl.Annotations))
		for k := range rl.Annotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, name := range keys {
			val := rl.Annotations[name]
			fmt.Fprintf(tw, "  %s:\t%s\n", name, val)
		}
	}
	tw.Flush()
	return buf.Bytes(), nil
}
