package proxy

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

type webSecurityHandler struct {
	next   http.Handler
	secure bool
}

func newWebSecurityHandler(next http.Handler, secure bool) http.Handler {
	return &webSecurityHandler{next: next, secure: secure}
}

func (h *webSecurityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("Content-Security-Policy", "base-uri 'self'; frame-ancestors 'none'; object-src 'none'")
	if h.secure {
		header.Set("Strict-Transport-Security", "max-age=31536000")
	}
	wrapped := &cookieHardeningWriter{ResponseWriter: w, secure: h.secure}
	h.next.ServeHTTP(wrapped, r)
	if !wrapped.wroteHeader {
		wrapped.WriteHeader(http.StatusOK)
	}
}

type cookieHardeningWriter struct {
	http.ResponseWriter
	secure      bool
	wroteHeader bool
}

func (w *cookieHardeningWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.hardenCookies()
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *cookieHardeningWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *cookieHardeningWriter) hardenCookies() {
	cookies := w.Header().Values("Set-Cookie")
	if len(cookies) == 0 {
		return
	}
	w.Header().Del("Set-Cookie")
	for _, cookie := range cookies {
		lower := strings.ToLower(cookie)
		if !strings.Contains(lower, "; samesite=") {
			cookie += "; SameSite=Lax"
		}
		if w.secure && !strings.Contains(lower, "; secure") {
			cookie += "; Secure"
		}
		w.Header().Add("Set-Cookie", cookie)
	}
}

func (w *cookieHardeningWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *cookieHardeningWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("response writer does not support hijacking")
}

func (w *cookieHardeningWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{w.ResponseWriter}, r)
}

func (w *cookieHardeningWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
