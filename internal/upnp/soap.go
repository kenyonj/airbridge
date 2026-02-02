package upnp

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
)

// ParseSOAPAction extracts the action name from a SOAPACTION header.
func ParseSOAPAction(sa string) string {
	sa = strings.Trim(sa, "\"")
	if i := strings.LastIndex(sa, "#"); i >= 0 {
		return sa[i+1:]
	}
	return sa
}

// WriteSOAPResponse writes a successful SOAP response.
func WriteSOAPResponse(w http.ResponseWriter, namespace, respName, inner string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:`)
	b.WriteString(respName)
	b.WriteString(` xmlns:u="`)
	b.WriteString(namespace)
	b.WriteString(`">`)
	b.WriteString(inner)
	b.WriteString(`</u:`)
	b.WriteString(respName)
	b.WriteString(`>
  </s:Body>
</s:Envelope>`)

	w.Write([]byte(b.String()))
}

// WriteSOAPError writes a SOAP error response.
func WriteSOAPError(w http.ResponseWriter, code int, desc string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.WriteHeader(500)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>UPnPError</faultstring>
      <detail>
        <UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
          <errorCode>`)
	b.WriteString(fmt.Sprintf("%d", code))
	b.WriteString(`</errorCode>
          <errorDescription>`)
	b.WriteString(desc)
	b.WriteString(`</errorDescription>
        </UPnPError>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`)

	w.Write([]byte(b.String()))
}

// XMLText extracts the text content of an XML element from raw bytes.
func XMLText(b []byte, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	s := string(b)
	i := strings.Index(s, open)
	if i < 0 {
		// Try with namespace prefix
		open = "<u:" + tag + ">"
		close = "</u:" + tag + ">"
		i = strings.Index(s, open)
		if i < 0 {
			return ""
		}
	}
	i += len(open)
	j := strings.Index(s[i:], close)
	if j < 0 {
		return ""
	}
	return html.UnescapeString(strings.TrimSpace(s[i : i+j]))
}

// ControllerID extracts the controller IP address from a request.
func ControllerID(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
