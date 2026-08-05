package api

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/ui"
)

// The console has no authentication. Any local account on the host can reach the console
// listener and can drive a root daemon. The section "The console has no authentication"
// of docs/specs/spec.md records the accepted risk and names four controls that reduce it.
// This file holds all four:
//
//  1. The listener binds a loopback address only, and StartConsole refuses and logs any
//     other address.
//  2. Every mutating route requires the header X-Hydrascale-Console: 1.
//  3. A request whose Origin header names another origin gets HTTP 403.
//  4. The daemon records one event for every mutating request on the console listener.
//
// Control 2 and control 3 stop a hostile web page, because a browser sets no custom
// header on a cross-origin form post and it sets the Origin header on a cross-origin
// request. Neither control stops a local account.
const (
	// ConsoleRequestHeader is the header that every mutating console route requires.
	ConsoleRequestHeader = "X-Hydrascale-Console"

	// ConsoleRequestHeaderValue is the one value that ConsoleRequestHeader may hold.
	ConsoleRequestHeaderValue = "1"

	// ConsoleContentSecurityPolicy is the policy on every console response. It allows the
	// console origin and an inline data image, and it allows no other source.
	ConsoleContentSecurityPolicy = "default-src 'self'; img-src 'self' data:"

	// EventConsoleRequest is the event kind of a mutating request on the console
	// listener.
	EventConsoleRequest = "console.request"
)

// consoleReadHeaderTimeout bounds how long a client may take to send its request head.
const consoleReadHeaderTimeout = 10 * time.Second

// ConsoleAddress returns the address that the console listener holds, such as
// 127.0.0.1:9443. It returns the empty string when the daemon opened no console listener.
func (s *Server) ConsoleAddress() string { return s.consoleAddress }

// StartConsole opens the console listener on bindAddress and serves the console files and
// the JSON API on it.
//
// StartConsole opens no listener and returns no error when enabled is false.
// StartConsole refuses a bind address that is not a loopback host and a port, it writes
// the reason into the log, and it returns that reason. It returns an error that names the
// port and the configuration key when the address is in use.
func (s *Server) StartConsole(enabled bool, bindAddress string) error {
	if !enabled {
		log.Print("api: console.enabled is false, so the daemon opens no console listener")
		return nil
	}

	if err := config.ValidateConsoleBindAddress(bindAddress); err != nil {
		log.Printf("api: refused the console bind address: %v", err)
		return err
	}

	ln, err := net.Listen("tcp", bindAddress)
	if err != nil {
		port := bindAddress
		if _, p, splitErr := net.SplitHostPort(bindAddress); splitErr == nil {
			port = p
		}
		return fmt.Errorf("failed to open the console listener on port %s: %w: set console.bind_address to a free port", port, err)
	}

	// The goroutine reads the local variable rather than the field, because Shutdown
	// writes the field and the race detector reports the pair.
	server := &http.Server{
		Handler:           s.consoleControls(s.consoleRoutes()),
		ReadHeaderTimeout: consoleReadHeaderTimeout,
	}
	s.consoleAddress = ln.Addr().String()
	s.consoleServer = server

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("api: console server error: %v", err)
		}
	}()

	log.Printf("api: the console listens on http://%s and it has no authentication: any local account on this host reaches it and drives the daemon, which runs as root", s.consoleAddress)
	return nil
}

// consoleRoutes returns the route set of the console listener: the JSON API under /api/,
// and the embedded console files everywhere else.
func (s *Server) consoleRoutes() http.Handler {
	mux := http.NewServeMux()
	// The JSON mux matches on the whole path, so the console listener answers the same
	// routes as the control socket. See FR-console-4.
	mux.Handle("/api/", s.mux)
	mux.Handle("/", ui.Handler())
	return mux
}

// consoleControls applies the four controls to every request on the console listener.
// It sets the content security policy on every response, it refuses a request from
// another origin, it refuses a mutating request that carries no console header, and it
// records one event for every mutating request.
func (s *Server) consoleControls(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", ConsoleContentSecurityPolicy)

		// A browser omits Origin on a same-origin navigation, and a command line client
		// omits it too, so an absent header is not evidence of a cross-origin request.
		if origin := r.Header.Get("Origin"); origin != "" && !s.isConsoleOrigin(origin) {
			log.Printf("api: refused a console request with the origin %q on %s %s", origin, r.Method, r.URL.Path)
			http.Error(w, "the Origin header does not name the console origin", http.StatusForbidden)
			return
		}

		if isMutatingMethod(r.Method) {
			if r.Header.Get(ConsoleRequestHeader) != ConsoleRequestHeaderValue {
				log.Printf("api: refused a console request with no %s header on %s %s", ConsoleRequestHeader, r.Method, r.URL.Path)
				http.Error(w, fmt.Sprintf("a mutating route requires the header %s: %s", ConsoleRequestHeader, ConsoleRequestHeaderValue), http.StatusForbidden)
				return
			}
			// The message holds the method and the path only. A request body carries an
			// auth key, and an event reaches the event log file and every reader of
			// GET /api/events. See SA-1.
			s.reconciler.RecordEvent(EventConsoleRequest, "", fmt.Sprintf("%s %s on the console listener", r.Method, r.URL.Path))
		}

		next.ServeHTTP(w, r)
	})
}

// isMutatingMethod reports whether method changes state.
func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isConsoleOrigin reports whether origin names the console itself.
//
// The console origin is HTTP on a loopback host and on the console port. isConsoleOrigin
// accepts the name localhost here, although ValidateConsoleBindAddress refuses it as a
// bind address: the browser has already connected to the loopback listener before it
// sends the header, so the name names this console. A page on another host that resolves
// to a loopback address still sends its own name in the header, and this check refuses
// it.
func (s *Server) isConsoleOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}

	_, consolePort, err := net.SplitHostPort(s.consoleAddress)
	if err != nil {
		return false
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || port != consolePort {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
