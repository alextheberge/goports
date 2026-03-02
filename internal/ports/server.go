// Package ports provides an HTTP adapter that exposes the port activity
// stream to other applications.  Clients can connect to `/events` to receive
// a Server-Sent Events (SSE) stream of PortActivity messages encoded as JSON.
//
// The server is purposely lightweight and optional; callers may start it on a
// local loopback address and choose the port they wish to publish on.
//
// Example usage:
//
//     srv, err := StartEventServer(":0")
//     if err != nil { ... }
//     fmt.Printf("listening on %s\n", srv.Addr)
//
// A test helper is provided below to programmatically start a server and
// obtain the bound address.

package ports


import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "math/big"
    "net"
    "net/http"
    "os"
    "strings"
    "time"
)

// openapiSpec holds the static JSON string returned by /openapi.json.  It is
// kept in a variable so tests and the CLI can reference the same data.
var openapiSpec = `{
    "openapi":"3.0.0",
    "info":{"title":"goports API","version":"0.1"},
    "paths":{
        "/events":{
            "get":{
                "description":"stream activity; supports SSE/WebSocket",
                "parameters":[
                    {"name":"token","in":"query","schema":{"type":"string"}},
                    {"name":"since","in":"query","schema":{"type":"string","format":"date-time"}}
                ]
            }
        },
        "/status":{
            "get":{
                "description":"current count of open ports",
                "responses":{
                    "200":{
                        "description":"port count",
                        "content":{
                            "application/json":{
                                "schema":{
                                    "type":"object",
                                    "properties":{
                                        "open":{"type":"integer"}
                                    }
                                }
                            }
                        }
                    }
                }
            }
        },
        "/ports":{
            "get":{
                "description":"snapshot of current listening ports with metadata",
                "responses":{
                    "200":{
                        "description":"array of port entries",
                        "content":{
                            "application/json":{
                                "schema":{
                                    "type":"array",
                                    "items":{
                                        "type":"object",
                                        "properties":{
                                            "Protocol":{"type":"string"},
                                            "Port":{"type":"integer"},
                                            "Host":{"type":"string"},
                                            "Pid":{"type":"integer"},
                                            "Name":{"type":"string"},
                                            "Cmdline":{"type":"string"},
                                            "AppBundle":{"type":"string"}
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        },
        "/history":{
            "get":{
                "description":"query past events",
                "parameters":[
                    {"name":"since","in":"query","schema":{"type":"string","format":"date-time"}},
                    {"name":"limit","in":"query","schema":{"type":"integer"}},
                    {"name":"protocol","in":"query","schema":{"type":"string"}},
                    {"name":"port","in":"query","schema":{"type":"integer"}},
                    {"name":"token","in":"query","schema":{"type":"string"}}
                ],
                "responses":{
                    "200":{
                        "description":"A list of port events",
                        "content":{
                            "application/json":{
                                "schema":{
                                    "type":"array",
                                    "items":{"$ref":"#/components/schemas/PortActivity"}
                                }
                            }
                        }
                    }
                }
            }
        },
        "/history/reset":{
            "post":{
                "description":"clear stored history",
                "responses":{
                    "204":{"description":"no content"}
                }
            }
        }
    },
    "components":{
        "schemas":{
            "PortActivity":{
                "type":"object",
                "properties":{
                    "Key":{"$ref":"#/components/schemas/PortKey"},
                    "Timestamp":{"type":"string","format":"date-time"},
                    "Open":{"type":"boolean"}
                }
            },
            "PortKey":{
                "type":"object",
                "properties":{
                    "Protocol":{"type":"string"},
                    "Port":{"type":"integer"}
                }
            }
        }
    }
}`

// OpenAPISpec returns the JSON served by /openapi.json.  This helper allows
// callers (CLI, tests) to retrieve the spec without making an HTTP request.
func OpenAPISpec() string {
    return openapiSpec
}

// StartEventServer begins listening on the provided network address and
// serves the `/events` endpoint (plus related paths such as `/history`,
// `/openapi.json` and `/swagger`).  The returned *http.Server should be
// gracefully Shutdown() when no longer needed; its Addr field reflects the
// actual listening address (useful when `addr` was ":0").
//
// If TLS mode is requested with the `tls:` prefix and no certificate files
// are provided via GOPORTS_TLS_CERT/GOPORTS_TLS_KEY, a temporary self-signed
// certificate pair is generated.  In that case the second return value is a
// cleanup function that will remove those files when called.  It is safe for
// callers to ignore the cleanup value when not using TLS.
func StartEventServer(addr string) (*http.Server, func(), error) {
    var ln net.Listener
    var err error
    tlsMode := false

    if strings.HasPrefix(addr, "unix:") {
        path := strings.TrimPrefix(addr, "unix:")
        os.Remove(path)
        ln, err = net.Listen("unix", path)
    } else if strings.HasPrefix(addr, "tls:") {
        tlsMode = true
        addr = strings.TrimPrefix(addr, "tls:")
        ln, err = net.Listen("tcp", addr)
    } else {
        ln, err = net.Listen("tcp", addr)
    }
    if err != nil {
        return nil, nil, err
    }

    mux := http.NewServeMux()

    // swagger UI served from embedded file
    mux.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
        if data, err := uiFiles.ReadFile("swagger.html"); err == nil {
            w.Header().Set("Content-Type", "text/html")
            w.Write(data)
        } else {
            http.NotFound(w, r)
        }
    })
    // serve static UI page at root
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        data, err := uiFiles.ReadFile("ui.html")
        if err != nil {
            http.Error(w, "internal error", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write(data)
    })

    // simple bearer-token authentication if GOPORTS_API_TOKEN is set
    authToken := os.Getenv("GOPORTS_API_TOKEN")
    authOK := func(r *http.Request) bool {
        if authToken == "" {
            return true
        }
        tok := r.URL.Query().Get("token")
        if tok == "" {
            auth := r.Header.Get("Authorization")
            if strings.HasPrefix(auth, "Bearer ") {
                tok = strings.TrimPrefix(auth, "Bearer ")
            }
        }
        return tok == authToken
    }

    // history endpoint returns JSON array of recent events. supports filtering
    // by protocol/port/since/limit.  requires auth if configured.
    mux.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
        if !authOK(r) {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        sinceStr := r.URL.Query().Get("since")
        var since time.Time
        if sinceStr != "" {
            if ts, err := time.Parse(time.RFC3339, sinceStr); err == nil {
                since = ts
            }
        }
        limit := 0
        if lim := r.URL.Query().Get("limit"); lim != "" {
            fmt.Sscanf(lim, "%d", &limit)
        }
        protoFilter := r.URL.Query().Get("protocol")
        portFilter := 0
        if pf := r.URL.Query().Get("port"); pf != "" {
            fmt.Sscanf(pf, "%d", &portFilter)
        }
        evts := History(since, limit)
        // apply protocol/port filter
        if protoFilter != "" || portFilter != 0 {
            var f []PortActivity
            for _, e := range evts {
                if protoFilter != "" && e.Key.Protocol != protoFilter {
                    continue
                }
                if portFilter != 0 && e.Key.Port != portFilter {
                    continue
                }
                f = append(f, e)
            }
            evts = f
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(evts)
    })

    // clear history buffer on request
    mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
        // unauthenticated; always accessible for UI clients
        data := AppsByPort()
        count := 0
        for _ = range data {
            // count distinct port keys
            count++
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]int{"open": count})
    })

    mux.HandleFunc("/ports", func(w http.ResponseWriter, r *http.Request) {
        // return a flat list of entries for UI consumption
        data := AppsByPort()
        type outEntry struct {
            Protocol  string
            Port      int
            Host      string
            Pid       int
            Name      string
            Cmdline   string
            AppBundle string
        }
        var out []outEntry
        for k, ents := range data {
            for _, e := range ents {
                out = append(out, outEntry{
                    Protocol:  k.Protocol,
                    Port:      k.Port,
                    Host:      e.Host,
                    Pid:       int(e.Pid),
                    Name:      e.Name,
                    Cmdline:   e.Cmdline,
                    AppBundle: e.AppBundle,
                })
            }
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(out)
    })

    // redirect chart.js source map to avoid 404 noise in UI
    mux.HandleFunc("/chart.umd.min.js.map", func(w http.ResponseWriter, r *http.Request) {
        http.Redirect(w, r, "https://cdn.jsdelivr.net/npm/chart.js/dist/chart.umd.min.js.map", http.StatusFound)
    })

    mux.HandleFunc("/history/reset", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "POST" {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        if !authOK(r) {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        clearHistory()
        w.WriteHeader(http.StatusNoContent)
    })

    mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        // helpers defined first
        mustJSON := func(v interface{}) string {
            b, _ := json.Marshal(v)
            return string(b)
        }
        handleWebSocket := func(w http.ResponseWriter, r *http.Request) {
            hj, ok := w.(http.Hijacker)
            if !ok {
                http.Error(w, "websocket unsupported", http.StatusInternalServerError)
                return
            }
            conn, bufrw, err := hj.Hijack()
            if err != nil {
                return
            }
            defer conn.Close()
            // perform simple websocket handshake
            fmt.Fprintf(bufrw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
            bufrw.Flush()
            ch := SubscribeActivity()
            for evt := range ch {
                data := mustJSON(evt)
                msg := fmt.Sprintf("%c\x00%s\xff", 0x00, data)
                conn.Write([]byte(msg))
            }
        }
        // WebSocket upgrade if requested
        if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
            handleWebSocket(w, r)
            return
        }
        if !authOK(r) {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        // optionally replay history starting from since
        if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
            if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
                for _, evt := range History(since, 0) {
                    fmt.Fprintf(w, "data: %s\n\n", mustJSON(evt))
                }
                if flusher, ok := w.(http.Flusher); ok {
                    flusher.Flush()
                }
            }
        }
        flusher, ok := w.(http.Flusher)
        if !ok {
            http.Error(w, "streaming unsupported", http.StatusInternalServerError)
            return
        }
        // per SSE spec
        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        // ensure headers are sent immediately
        if flusher, ok := w.(http.Flusher); ok {
            flusher.Flush()
        }

        notify := w.(http.CloseNotifier).CloseNotify()
        ch := SubscribeActivity()
        for {
            select {
            case evt := <-ch:
                data, _ := json.Marshal(evt)
                fmt.Fprintf(w, "data: %s\n\n", data)
                flusher.Flush()
            case <-notify:
                return
            }
        }
    })

    // expose simple OpenAPI spec
    mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(openapiSpec))
    })

    srv := &http.Server{Addr: ln.Addr().String(), Handler: mux}

    var cleanup func()
    // if TLS mode requested we may either use provided cert/key or generate
    // a temporary self-signed pair.

    // if TLS mode requested we may either use provided cert/key or generate
    // a temporary self-signed pair.
    certFile := os.Getenv("GOPORTS_TLS_CERT")
    keyFile := os.Getenv("GOPORTS_TLS_KEY")
    if tlsMode {
        if certFile == "" || keyFile == "" {
            // generate ephemeral certs
            certFile, keyFile, err = generateSelfSignedTLS()
            if err != nil {
                return nil, nil, err
            }
            // schedule removal of generated files
            cleanup = func() {
                os.Remove(certFile)
                os.Remove(keyFile)
            }
        }
        go srv.ServeTLS(ln, certFile, keyFile) // nolint:errcheck
    } else {
        if certFile != "" && keyFile != "" {
            go srv.ServeTLS(ln, certFile, keyFile) // nolint:errcheck
        } else {
            go srv.Serve(ln) // nolint:errcheck
        }
    }
    return srv, cleanup, nil
}

// generateSelfSignedTLS creates a temporary self-signed certificate and key,
// returning their filenames.  Caller is responsible for removing them if
// desired.
func generateSelfSignedTLS() (certFile, keyFile string, err error) {
    priv, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        return "", "", err
    }
    tmpl := x509.Certificate{
        SerialNumber: big.NewInt(time.Now().UnixNano()),
        Subject: pkix.Name{
            CommonName: "localhost",
        },
        NotBefore: time.Now().Add(-time.Hour),
        NotAfter:  time.Now().Add(24 * time.Hour),
        KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
        DNSNames:    []string{"localhost"},
    }
    certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
    if err != nil {
        return "", "", err
    }
    cf, err := os.CreateTemp("", "goports-cert-*.pem")
    if err != nil {
        return "", "", err
    }
    kf, err := os.CreateTemp("", "goports-key-*.pem")
    if err != nil {
        return "", "", err
    }
    // write cert PEM
    if err := pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
        return "", "", err
    }
    // write key PEM
    keyBytes := x509.MarshalPKCS1PrivateKey(priv)
    if err := pem.Encode(kf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}); err != nil {
        return "", "", err
    }
    cf.Close()
    kf.Close()
    return cf.Name(), kf.Name(), nil
}

// Example test helper: Start a server, return listener address and shutdown
// function.
func startTestServer() (addr string, shutdown func(), err error) {
    srv, cleanup, err := StartEventServer(":0")
    if err != nil {
        return "", nil, err
    }
    shutdown = func() {
        _ = srv.Close()
        if cleanup != nil {
            cleanup()
        }
    }
    return srv.Addr, shutdown, nil
}

// OpenAPISpec returns the JSON served by /openapi.json.  This helper allows
// callers (CLI, tests) to retrieve the spec without making an HTTP request.
