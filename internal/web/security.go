package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	cookiePrefix = "hp_token_"
	bearerPrefix = "Bearer "
	cspPolicy    = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

func NewToken(r io.Reader) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	var b [32]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func AllowedHosts(addr string) []string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return []string{addr}
	}
	return []string{addr, net.JoinHostPort("localhost", port), net.JoinHostPort("127.0.0.1", port)}
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		switch {
		case !s.hostAllowed(r.Host):
			writeError(w, http.StatusForbidden, "host not allowed")
		case s.exchangeToken(w, r):
		case !s.authorized(r):
			writeError(w, http.StatusUnauthorized, "token required")
		case isSafe(r):
			next.ServeHTTP(w, r)
		case !originAllowed(r):
			writeError(w, http.StatusForbidden, "origin not allowed")
		case !isJSON(r):
			writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", cspPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
}

func (s *Server) hostAllowed(host string) bool {
	_, ok := s.hosts[strings.ToLower(host)]
	return ok
}

func (s *Server) exchangeToken(w http.ResponseWriter, r *http.Request) bool {
	if !isSafe(r) || !r.URL.Query().Has("token") {
		return false
	}
	if !s.tokenMatches(r.URL.Query().Get("token")) {
		writeError(w, http.StatusUnauthorized, "bad token")
		return true
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(r.Host), Value: s.token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/", http.StatusFound)
	return true
}

func cookieName(host string) string {
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		return cookiePrefix + "default"
	}
	return cookiePrefix + port
}

func (s *Server) authorized(r *http.Request) bool {
	if c, err := r.Cookie(cookieName(r.Host)); err == nil && s.tokenMatches(c.Value) {
		return true
	}
	bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), bearerPrefix)
	return ok && s.tokenMatches(bearer)
}

func (s *Server) tokenMatches(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func isSafe(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead
}

func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func isJSON(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == "application/json"
}
