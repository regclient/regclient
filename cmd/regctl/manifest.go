package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/containerd/containerd/platforms"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/regclient"
	"github.com/regclient/regclient/regclient/manifest"
	"github.com/regclient/regclient/regclient/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var manifestCmd = &cobra.Command{
	Use:   "manifest <cmd>",
	Short: "manage manifests",
}

var manifestDeleteCmd = &cobra.Command{
	Use:     "delete <image_ref>",
	Aliases: []string{"del", "rm", "remove"},
	Short:   "delete a manifest",
	Args:    cobra.RangeArgs(1, 1),
	RunE:    runManifestDelete,
}

var manifestDigestCmd = &cobra.Command{
	Use:   "digest <image_ref>",
	Short: "retrieve digest of manifest",
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runManifestDigest,
}

var manifestGetCmd = &cobra.Command{
	Use:   "get <image_ref>",
	Short: "retrieve manifest or manifest list",
	Long:  `Shows the manifest or manifest list of the specified image.`,
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runManifestGet,
}

var manifestPutCmd = &cobra.Command{
	Use:   "put <image_ref>",
	Short: "push manifest or manifest list",
	Long:  `Pushes a manifest or manifest list to a repository.`,
	Args:  cobra.RangeArgs(1, 1),
	RunE:  runManifestPut,
}

var manifestOpts struct {
	list        bool
	platform    string
	requireList bool
	format      string
	contentType string
}

func init() {
	manifestDigestCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", false, "Do not resolve platform from manifest list (recommended)")
	manifestDigestCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	manifestDigestCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")

	manifestGetCmd.Flags().BoolVarP(&manifestOpts.list, "list", "", false, "Output manifest list if available")
	manifestGetCmd.Flags().StringVarP(&manifestOpts.platform, "platform", "p", "", "Specify platform (e.g. linux/amd64)")
	manifestGetCmd.Flags().BoolVarP(&manifestOpts.requireList, "require-list", "", false, "Fail if manifest list is not received")
	manifestGetCmd.Flags().StringVarP(&manifestOpts.format, "format", "", "{{printPretty .}}", "Format output with go template syntax")

	manifestPutCmd.Flags().StringVarP(&manifestOpts.contentType, "content-type", "t", "", "Specify content-type (e.g. application/vnd.docker.distribution.manifest.v2+json)")
	manifestPutCmd.MarkFlagRequired("content-type")

	manifestCmd.AddCommand(manifestDeleteCmd)
	manifestCmd.AddCommand(manifestDigestCmd)
	manifestCmd.AddCommand(manifestGetCmd)
	manifestCmd.AddCommand(manifestPutCmd)
	rootCmd.AddCommand(manifestCmd)
}

func getManifest(rc regclient.RegClient, ref types.Ref) (manifest.Manifest, error) {
	m, err := rc.ManifestGet(context.Background(), ref)
	if err != nil {
		return m, err
	}

	// add warning if not list and list required or platform requested
	if !m.IsList() && manifestOpts.requireList {
		log.Warn("Manifest list unavailable")
		return m, ErrNotFound
	}
	if !m.IsList() && manifestOpts.platform != "" {
		log.Info("Manifest list unavailable, ignoring platform flag")
	}

	// retrieve the specified platform from the manifest list
	if m.IsList() && !manifestOpts.list && !manifestOpts.requireList {
		desc, err := getPlatformDesc(rc, m)
		ref.Digest = desc.Digest.String()
		m, err = rc.ManifestGet(context.Background(), ref)
		if err != nil {
			return m, fmt.Errorf("Failed to pull platform specific digest: %w", err)
		}
	}
	return m, nil
}

func getPlatformDesc(rc regclient.RegClient, m manifest.Manifest) (*ociv1.Descriptor, error) {
	var desc *ociv1.Descriptor
	var err error
	if !m.IsList() {
		return desc, fmt.Errorf("%w: manifest is not a list", ErrInvalidInput)
	}
	if !m.IsSet() {
		m, err = rc.ManifestGet(context.Background(), m.GetRef())
		if err != nil {
			return desc, err
		}
	}

	var plat ociv1.Platform
	if manifestOpts.platform != "" {
		plat, err = platforms.Parse(manifestOpts.platform)
		if err != nil {
			log.WithFields(logrus.Fields{
				"platform": manifestOpts.platform,
				"err":      err,
			}).Warn("Could not parse platform")
		}
	}
	if plat.OS == "" {
		plat = platforms.DefaultSpec()
	}
	desc, err = m.GetPlatformDesc(&plat)
	if err != nil {
		pl, _ := m.GetPlatformList()
		var ps []string
		for _, p := range pl {
			ps = append(ps, platforms.Format(*p))
		}
		log.WithFields(logrus.Fields{
			"platform":  platforms.Format(plat),
			"err":       err,
			"platforms": strings.Join(ps, ", "),
		}).Warn("Platform could not be found in manifest list")
		return desc, ErrNotFound
	}
	log.WithFields(logrus.Fields{
		"platform": platforms.Format(plat),
		"digest":   desc.Digest.String(),
	}).Debug("Found platform specific digest in manifest list")
	return desc, nil
}

func runManifestDelete(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image digest")

	err = rc.ManifestDelete(context.Background(), ref)
	if err != nil {
		return err
	}
	return nil
}

func runManifestDigest(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	log.WithFields(logrus.Fields{
		"host": ref.Registry,
		"repo": ref.Repository,
		"tag":  ref.Tag,
	}).Debug("Image digest")

	// attempt to request only the headers, avoids Docker Hub rate limits
	m, err := rc.ManifestHead(context.Background(), ref)
	if err != nil {
		return err
	}

	// add warning if not list and list required or platform requested
	if !m.IsList() && manifestOpts.requireList {
		log.Warn("Manifest list unavailable")
		return ErrNotFound
	}
	if !m.IsList() && manifestOpts.platform != "" {
		log.Info("Manifest list unavailable, ignoring platform flag")
	}

	// retrieve the specified platform from the manifest list
	for m.IsList() && !manifestOpts.list && !manifestOpts.requireList {
		desc, err := getPlatformDesc(rc, m)
		ref.Digest = desc.Digest.String()
		m, err = rc.ManifestHead(context.Background(), ref)
		if err != nil {
			return fmt.Errorf("Failed retrieving platform specific digest: %w", err)
		}
	}

	fmt.Println(m.GetDigest().String())
	return nil
}

func runManifestGet(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()

	m, err := getManifest(rc, ref)
	if err != nil {
		return err
	}

	switch manifestOpts.format {
	case "raw":
		manifestOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}{{printf \"\\n%s\" .RawBody}}"
	case "rawBody", "raw-body", "body":
		manifestOpts.format = "{{printf \"%s\" .RawBody}}"
	case "rawHeaders", "raw-headers", "headers":
		manifestOpts.format = "{{ range $key,$vals := .RawHeaders}}{{range $val := $vals}}{{printf \"%s: %s\\n\" $key $val }}{{end}}{{end}}"
	}
	return template.Writer(os.Stdout, manifestOpts.format, m, template.WithFuncs(regclient.TemplateFuncs))
}

func runManifestPut(cmd *cobra.Command, args []string) error {
	ref, err := types.NewRef(args[0])
	if err != nil {
		return err
	}
	rc := newRegClient()
	raw, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	rcM, err := manifest.New(manifestOpts.contentType, raw, ref, nil)
	if err != nil {
		return err
	}

	return rc.ManifestPut(context.Background(), ref, rcM)
}
