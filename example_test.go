package regclient_test

import (
	"context"
	"fmt"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/types/ref"
)

func ExampleNew() {
	ctx := context.Background()
	// use config.Host to provide registry logins, TLS, and other registry settings
	exHostLocal := config.Host{
		Name: "registry.example.org:5000",
		TLS:  config.TLSDisabled,
		User: "exUser",
		Pass: "exPass",
	}
	exHostDH := config.Host{
		Name: "docker.io",
		User: "dhUser",
		Pass: "dhPass",
	}
	// define a regclient with desired options
	rc := regclient.New(
		regclient.WithConfigHosts([]config.Host{exHostLocal, exHostDH}),
		regclient.WithDockerCerts(),
		regclient.WithDockerCreds(),
		regclient.WithUserAgent("regclient/example"),
	)
	// create a reference for an image
	r, err := ref.New("ghcr.io/regclient/regctl:latest")
	if err != nil {
		fmt.Printf("failed to create ref: %v\n", err)
		return
	}
	defer rc.Close(ctx, r)
	// get a manifest (or call other regclient methods)
	m, err := rc.ManifestGet(ctx, r)
	if err != nil {
		fmt.Printf("failed to get manifest: %v\n", err)
		return
	}
	fmt.Println(m.GetDescriptor().MediaType)
	// Output: application/vnd.oci.image.index.v1+json
}
