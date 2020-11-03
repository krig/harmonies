package server

import (
	"bufio"
	"crypto/tls"
	"log"
	"net"
	"net/http"
)

// ListenAndServeWithRedirect enables seamless HTTP -> HTTPS redirect
// on the same port. This is useful for Hawk so that if someone
// accesses the :7630 port over HTTP, it'll automagically redirect to
// HTTPS.
func ListenAndServeWithRedirect(addr string, cert string, key string) {
	config := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	// TODO: investigate why this is needed
	if config.NextProtos == nil {
		config.NextProtos = []string{"http1/1"}
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(cert, key)
	if err != nil {
		log.Fatal(err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	listener := &splitListener{
		Listener: ln,
		config:   config,
	}

	srv := &http.Server{
		Addr:      addr,
		TLSConfig: config,
	}
	srv.Serve(listener)
}

type splitListener struct {
	net.Listener
	config *tls.Config
}

func (l *splitListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	bconn := &conn{
		Conn: c,
		buf:  bufio.NewReader(c),
	}

	// inspect the first bytes to see if it is HTTPS
	hdr, err := bconn.buf.Peek(6)
	if err != nil {
		log.Printf("Short %s: %s\n", c.RemoteAddr().String(), err.Error())
		// couldn't peek, assume it's HTTPS
		return tls.Server(bconn, l.config), nil
	}

	// SSL 3.0 or TLS 1.0, 1.1 and 1.2
	if hdr[0] == 0x16 && hdr[1] == 0x3 && hdr[5] == 0x1 {
		return tls.Server(bconn, l.config), nil
		// SSL 2
	} else if hdr[0] == 0x80 {
		return tls.Server(bconn, l.config), nil
	}
	return bconn, nil
}

type conn struct {
	net.Conn
	buf *bufio.Reader
}

func (c *conn) Read(b []byte) (int, error) {
	return c.buf.Read(b)
}
