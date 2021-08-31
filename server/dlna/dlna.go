package dlna

import (
	"log"

	"server/dlna/serve/dlna"
)

var (
	dlnaServer *dlna.Server
)

func Start() {
	dlnaServer = dlna.NewServer()
	if err := dlnaServer.Serve(); err != nil {
		log.Println(err)
	}
	dlnaServer.Wait()
}

func Stop() {
	if dlnaServer != nil {
		dlnaServer.Close()
		dlnaServer = nil
	}
}
