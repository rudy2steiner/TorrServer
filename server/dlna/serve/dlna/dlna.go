package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"server/dlna/serve/dlna/data"
	"server/dlna/serve/dlna/soap"
	"server/dlna/serve/dlna/ssdp"
	"server/dlna/serve/dlna/upnp"
)

const (
	serverField       = "Linux/3.4 DLNADOC/1.50 UPnP/1.0 DMS/1.0"
	rootDescPath      = "/rootDesc.xml"
	resPath           = "/r/"
	serviceControlURL = "/ctl"
)

type server struct {
	// The service SOAP handler keyed by service URN.
	services map[string]UPnPService

	Interfaces []net.Interface

	HTTPConn       net.Listener
	httpListenAddr string
	handler        http.Handler

	RootDeviceUUID string

	FriendlyName string

	// For waiting on the listener to close
	waitChan chan struct{}

	// Time interval between SSPD announces
	AnnounceInterval time.Duration
}

func newServer() *server {
	friendlyName := makeDefaultFriendlyName()

	s := &server{
		AnnounceInterval: 10 * time.Second,
		FriendlyName:     friendlyName,
		RootDeviceUUID:   makeDeviceUUID(friendlyName),
		Interfaces:       listInterfaces(),

		httpListenAddr: ":9080",
	}

	s.services = map[string]UPnPService{
		"ContentDirectory": &contentDirectoryService{
			server: s,
		},
		"ConnectionManager": &connectionManagerService{
			server: s,
		},
		"X_MS_MediaReceiverRegistrar": &mediaReceiverRegistrarService{
			server: s,
		},
	}

	// Setup the various http routes.
	r := http.NewServeMux()
	r.Handle(resPath, http.StripPrefix(resPath,
		http.HandlerFunc(s.resourceHandler)))
	if true {
		r.Handle(rootDescPath, traceLogging(http.HandlerFunc(s.rootDescHandler)))
		r.Handle(serviceControlURL, traceLogging(http.HandlerFunc(s.serviceControlHandler)))
	} else {
		r.HandleFunc(rootDescPath, s.rootDescHandler)
		r.HandleFunc(serviceControlURL, s.serviceControlHandler)
	}
	r.Handle("/static/", http.StripPrefix("/static/",
		withHeader("Cache-Control", "public, max-age=86400",
			http.FileServer(data.Assets))))
	s.handler = logging(withHeader("Server", serverField, r))

	return s
}

// UPnPService is the interface for the SOAP service.
type UPnPService interface {
	Handle(action string, argsXML []byte, r *http.Request) (respArgs map[string]string, err error)
	Subscribe(callback []*url.URL, timeoutSeconds int) (sid string, actualTimeout int, err error)
	Unsubscribe(sid string) error
}

// Formats the server as a string (used for logging.)
func (s *server) String() string {
	return fmt.Sprintf("DLNA server on %v", s.httpListenAddr)
}

// Renders the root device descriptor.
func (s *server) rootDescHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := data.GetTemplate()
	if err != nil {
		serveError(s, w, "Failed to load root descriptor template", err)
		return
	}

	buffer := new(bytes.Buffer)
	err = tmpl.Execute(buffer, s)
	if err != nil {
		serveError(s, w, "Failed to render root descriptor XML", err)
		return
	}

	w.Header().Set("content-type", `text/xml; charset="utf-8"`)
	w.Header().Set("cache-control", "private, max-age=60")
	w.Header().Set("content-length", strconv.FormatInt(int64(buffer.Len()), 10))
	_, err = buffer.WriteTo(w)
	if err != nil {
		// Network error
		log.Printf("%v Error writing rootDesc: %v", s, err)
	}
}

// Handle a service control HTTP request.
func (s *server) serviceControlHandler(w http.ResponseWriter, r *http.Request) {
	soapActionString := r.Header.Get("SOAPACTION")
	soapAction, err := parseActionHTTPHeader(soapActionString)
	if err != nil {
		serveError(s, w, "Could not parse SOAPACTION header", err)
		return
	}
	var env soap.Envelope
	if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
		serveError(s, w, "Could not parse SOAP request body", err)
		return
	}

	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("Ext", "")
	soapRespXML, code := func() ([]byte, int) {
		respArgs, err := s.soapActionResponse(soapAction, env.Body.Action, r)
		if err != nil {
			log.Printf("%v Error invoking %v: %v", s, soapAction, err)
			upnpErr := upnp.ConvertError(err)
			return mustMarshalXML(soap.NewFault("UPnPError", upnpErr)), http.StatusInternalServerError
		}
		return marshalSOAPResponse(soapAction, respArgs), http.StatusOK
	}()
	bodyStr := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" standalone="yes"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>%s</s:Body></s:Envelope>`, soapRespXML)
	w.WriteHeader(code)
	if _, err := w.Write([]byte(bodyStr)); err != nil {
		log.Printf("%v Error writing response: %v", s, err)
	}
}

// Handle a SOAP request and return the response arguments or UPnP error.
func (s *server) soapActionResponse(sa upnp.SoapAction, actionRequestXML []byte, r *http.Request) (map[string]string, error) {
	service, ok := s.services[sa.Type]
	if !ok {
		// TODO: What's the invalid service error?
		return nil, upnp.Errorf(upnp.InvalidActionErrorCode, "Invalid service: %s", sa.Type)
	}
	return service.Handle(sa.Action, actionRequestXML, r)
}

// Serves actual resources (media files).
func (s *server) resourceHandler(w http.ResponseWriter, r *http.Request) {
	remotePath := r.URL.Path
	node, err := os.Stat(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(node.Size(), 10))

	// add some DLNA specific headers
	if r.Header.Get("getContentFeatures.dlna.org") != "" {
		w.Header().Set("contentFeatures.dlna.org", ContentFeatures{
			SupportRange: true,
		}.String())
	}
	w.Header().Set("transferMode.dlna.org", "Streaming")

	in, err := os.OpenFile(r.URL.Path, os.O_RDONLY, 0666)
	if err != nil {
		serveError(node, w, "Could not open resource", err)
		return
	}
	http.ServeContent(w, r, remotePath, node.ModTime(), in)
}

// Serve runs the server - returns the error only if
// the listener was not started; does not block, so
// use s.Wait() to block on the listener indefinitely.
func (s *server) Serve() (err error) {
	if s.HTTPConn == nil {
		s.HTTPConn, err = net.Listen("tcp", s.httpListenAddr)
		if err != nil {
			return
		}
	}

	go func() {
		s.startSSDP()
	}()

	go func() {
		log.Printf("Serving HTTP on %s", s.HTTPConn.Addr().String())

		err = s.serveHTTP()
		if err != nil {
			log.Printf("Error on serving HTTP server: %v", err)
		}
	}()

	return nil
}

// Wait blocks while the listener is open.
func (s *server) Wait() {
	<-s.waitChan
}

func (s *server) Close() {
	err := s.HTTPConn.Close()
	if err != nil {
		log.Printf("Error closing HTTP server: %v", err)
		return
	}
	close(s.waitChan)
}

// Run SSDP (multicast for server discovery) on all interfaces.
func (s *server) startSSDP() {
	active := 0
	stopped := make(chan struct{})
	for _, intf := range s.Interfaces {
		active++
		go func(intf2 net.Interface) {
			defer func() {
				stopped <- struct{}{}
			}()
			s.ssdpInterface(intf2)
		}(intf)
	}
	for active > 0 {
		<-stopped
		active--
	}
}

// Run SSDP server on an interface.
func (s *server) ssdpInterface(intf net.Interface) {
	// Figure out which HTTP location to advertise based on the interface IP.
	advertiseLocationFn := func(ip net.IP) string {
		url := url.URL{
			Scheme: "http",
			Host: (&net.TCPAddr{
				IP:   ip,
				Port: s.HTTPConn.Addr().(*net.TCPAddr).Port,
			}).String(),
			Path: rootDescPath,
		}
		return url.String()
	}

	// Note that the devices and services advertised here via SSDP should be
	// in agreement with the rootDesc XML descriptor that is defined above.
	ssdpServer := ssdp.Server{
		Interface: intf,
		Devices: []string{
			"urn:schemas-upnp-org:device:MediaServer:1"},
		Services: []string{
			"urn:schemas-upnp-org:service:ContentDirectory:1",
			"urn:schemas-upnp-org:service:ConnectionManager:1",
			"urn:microsoft.com:service:X_MS_MediaReceiverRegistrar:1"},
		Location:       advertiseLocationFn,
		Server:         serverField,
		UUID:           s.RootDeviceUUID,
		NotifyInterval: s.AnnounceInterval,
	}

	// An interface with these flags should be valid for SSDP.
	const ssdpInterfaceFlags = net.FlagUp | net.FlagMulticast

	if err := ssdpServer.Init(); err != nil {
		if intf.Flags&ssdpInterfaceFlags != ssdpInterfaceFlags {
			// Didn't expect it to work anyway.
			return
		}
		if strings.Contains(err.Error(), "listen") {
			// OSX has a lot of dud interfaces. Failure to create a socket on
			// the interface are what we're expecting if the interface is no
			// good.
			return
		}
		log.Printf("%v Error creating ssdp server on %s: %s", s, intf.Name, err)
		return
	}
	defer ssdpServer.Close()
	log.Printf("%v Started SSDP on %v", s, intf.Name)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := ssdpServer.Serve(); err != nil {
			log.Printf("%v %q: %q\n", s, intf.Name, err)
		}
	}()
	select {
	case <-s.waitChan:
		// Returning will close the server.
	case <-stopped:
	}
}

func (s *server) serveHTTP() error {
	srv := &http.Server{
		Handler: s.handler,
	}
	err := srv.Serve(s.HTTPConn)
	select {
	case <-s.waitChan:
		return nil
	default:
		return err
	}
}
