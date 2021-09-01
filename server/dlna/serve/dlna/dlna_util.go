package dlna

import (
	"crypto/md5"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"server/dlna/serve/dlna/soap"
	"server/dlna/serve/dlna/upnp"
)

// Return a default "friendly name" for the server.
func makeDefaultFriendlyName() string {
	hostName, err := os.Hostname()
	if err != nil {
		hostName = ""
	} else {
		if hostName == "localhost" { // useless host, use 1st IP
			ifaces, err := net.Interfaces()
			if err != nil {
				return "TorrServer" + " (" + hostName + ")"
			}
			var list []string
			for _, i := range ifaces {
				addrs, _ := i.Addrs()
				if i.Flags&net.FlagUp == net.FlagUp {
					for _, addr := range addrs {
						var ip net.IP
						switch v := addr.(type) {
						case *net.IPNet:
							ip = v.IP
						case *net.IPAddr:
							ip = v.IP
						}
						if !ip.IsLoopback() {
							list = append(list, ip.String())
						}
					}
				}
			}
			if len(list) > 0 {
				hostName = list[0]
			}
		}
		hostName = " (" + hostName + ")"
	}
	return "TorrServer" + hostName
}

func makeDeviceUUID(unique string) string {
	h := md5.New()
	if _, err := io.WriteString(h, unique); err != nil {
		log.Panicf("makeDeviceUUID write failed: %s", err)
	}
	buf := h.Sum(nil)
	return upnp.FormatUUID(buf)
}

// Get all available active network interfaces.
func listInterfaces() []net.Interface {
	ifs, err := net.Interfaces()
	if err != nil {
		log.Printf("list network interfaces: %v", err)
		return []net.Interface{}
	}

	var active []net.Interface
	for _, intf := range ifs {
		if intf.Flags&net.FlagUp != 0 && intf.Flags&net.FlagMulticast != 0 && intf.MTU > 0 {
			active = append(active, intf)
		}
	}
	return active
}

func didlLite(chardata string) string {
	return `<DIDL-Lite` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"` +
		` xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` +
		chardata +
		`</DIDL-Lite>`
}

func mustMarshalXML(value interface{}) []byte {
	ret, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		log.Panicf("mustMarshalXML failed to marshal %v: %s", value, err)
	}
	return ret
}

// Marshal SOAP response arguments into a response XML snippet.
func marshalSOAPResponse(sa upnp.SoapAction, args map[string]string) []byte {
	soapArgs := make([]soap.Arg, 0, len(args))
	for argName, value := range args {
		soapArgs = append(soapArgs, soap.Arg{
			XMLName: xml.Name{Local: argName},
			Value:   value,
		})
	}
	return []byte(fmt.Sprintf(`<u:%[1]sResponse xmlns:u="%[2]s">%[3]s</u:%[1]sResponse>`,
		sa.Action, sa.ServiceURN.String(), mustMarshalXML(soapArgs)))
}

var serviceURNRegexp = regexp.MustCompile(`:service:(\w+):(\d+)$`)

func parseServiceType(s string) (ret upnp.ServiceURN, err error) {
	matches := serviceURNRegexp.FindStringSubmatch(s)
	if matches == nil {
		err = errors.New(s)
		return
	}
	if len(matches) != 3 {
		log.Panicf("Invalid serviceURNRegexp ?")
	}
	ret.Type = matches[1]
	ret.Version, err = strconv.ParseUint(matches[2], 0, 0)
	return
}

func parseActionHTTPHeader(s string) (ret upnp.SoapAction, err error) {
	if s[0] != '"' || s[len(s)-1] != '"' {
		return
	}
	s = s[1 : len(s)-1]
	hashIndex := strings.LastIndex(s, "#")
	if hashIndex == -1 {
		return
	}
	ret.Action = s[hashIndex+1:]
	ret.ServiceURN, err = parseServiceType(s[:hashIndex])
	return
}

type loggingResponseWriter struct {
	http.ResponseWriter
	request   *http.Request
	committed bool
}

func (lrw *loggingResponseWriter) logRequest(code int, err interface{}) {
	if err == nil {
		err = ""
	}

	log.Printf("%v %s %s %d %s %s", lrw.request.URL,
		lrw.request.RemoteAddr, lrw.request.Method, code,
		lrw.request.Header.Get("SOAPACTION"), err)
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.committed = true
	lrw.logRequest(code, nil)
	lrw.ResponseWriter.WriteHeader(code)
}

// HTTP handler that logs requests and any errors or panics.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := &loggingResponseWriter{ResponseWriter: w, request: r}
		defer func() {
			err := recover()
			if err != nil {
				if !lrw.committed {
					lrw.logRequest(http.StatusInternalServerError, err)
					http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
				} else {
					// Too late to send the error to client, but at least log it.
					log.Printf("%v Recovered panic: %v", r.URL.Path, err)
				}
			}
		}()
		next.ServeHTTP(lrw, r)
	})
}

// HTTP handler that logs complete request and response bodies for debugging.
// Error recovery and general request logging are left to logging().
func traceLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			serveError(nil, w, "error dumping request", err)
			return
		}
		log.Printf("%s", dump)

		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)

		dump, err = httputil.DumpResponse(recorder.Result(), true)
		if err != nil {
			// log the error but ignore it
			log.Printf("error dumping response: %v", err)
		} else {
			log.Printf("%s", dump)
		}

		// copy from recorder to the real response writer
		for k, v := range recorder.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(recorder.Code)
		_, err = recorder.Body.WriteTo(w)
		if err != nil {
			// Network error
			log.Printf("Error writing response: %v", err)
		}
	})
}

// HTTP handler that sets headers.
func withHeader(name string, value string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(name, value)
		next.ServeHTTP(w, r)
	})
}

// serveError returns an http.StatusInternalServerError and logs the error
func serveError(what interface{}, w http.ResponseWriter, text string, err error) {
	log.Printf("%v %s: %v", what, text, err)
	http.Error(w, text+".", http.StatusInternalServerError)
}

// Splits a path into (root, ext) such that root + ext == path, and ext is empty
// or begins with a period.  Extended version of path.Ext().
func splitExt(path string) (string, string) {
	for i := len(path) - 1; i >= 0 && path[i] != '/'; i-- {
		if path[i] == '.' {
			return path[:i], path[i:]
		}
	}
	return path, ""
}
