package main

import (
	"github.com/regclient/regclient/regclient"
)

func main() {
	regclient.ConfigDir = ".regctl"
	regclient.ConfigEnv = "REGCLI_CONFIG"
	Execute()
}
