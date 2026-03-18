package controller

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusResponseWriterWriteDefaultStatusAndBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusResponseWriter(rec)

	n, err := sw.Write([]byte("ok"))
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != 2 {
		t.Fatalf("unexpected write size: got %d, want %d", n, 2)
	}
	if sw.Status() != http.StatusOK {
		t.Fatalf("unexpected status: got %d, want %d", sw.Status(), http.StatusOK)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected recorder status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("unexpected body: got %q, want %q", rec.Body.String(), "ok")
	}
}

func TestStatusResponseWriterWriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusResponseWriter(rec)

	sw.WriteHeader(http.StatusCreated)
	sw.WriteHeader(http.StatusInternalServerError)

	if sw.Status() != http.StatusCreated {
		t.Fatalf("unexpected status: got %d, want %d", sw.Status(), http.StatusCreated)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected recorder status: got %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestStatusResponseWriterReadFromCopiesBody(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusResponseWriter(rec)

	n, err := sw.ReadFrom(strings.NewReader("abc"))
	if err != nil {
		t.Fatalf("unexpected ReadFrom error: %v", err)
	}
	if n != 3 {
		t.Fatalf("unexpected copied bytes: got %d, want %d", n, 3)
	}
	if rec.Body.String() != "abc" {
		t.Fatalf("unexpected body: got %q, want %q", rec.Body.String(), "abc")
	}
}

func TestStatusResponseWriterHijackSwitchesState(t *testing.T) {
	hw := newTestHijackResponseWriter(t)
	sw := newStatusResponseWriter(hw)

	conn, rw, err := sw.Hijack()
	if err != nil {
		t.Fatalf("unexpected hijack error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected hijacked connection")
	}
	if rw == nil {
		t.Fatal("expected hijacked read writer")
	}
	if sw.Status() != http.StatusSwitchingProtocols {
		t.Fatalf("unexpected status after hijack: got %d, want %d", sw.Status(), http.StatusSwitchingProtocols)
	}
	if !sw.WroteHeader() {
		t.Fatal("expected wroteHeader=true after hijack")
	}
	if !sw.IsHijacked() {
		t.Fatal("expected hijacked=true after hijack")
	}

	if _, err = sw.Write([]byte("x")); !errors.Is(err, http.ErrHijacked) {
		t.Fatalf("expected http.ErrHijacked, got %v", err)
	}
}

func TestStatusResponseWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusResponseWriter(rec)

	if got := sw.Unwrap(); got != rec {
		t.Fatalf("unexpected unwrap target: got %T, want %T", got, rec)
	}
}

type testHijackResponseWriter struct {
	header http.Header
	conn   net.Conn
	rw     *bufio.ReadWriter
}

func newTestHijackResponseWriter(t *testing.T) *testHijackResponseWriter {
	t.Helper()

	conn1, conn2 := net.Pipe()
	t.Cleanup(func() {
		_ = conn1.Close()
		_ = conn2.Close()
	})

	return &testHijackResponseWriter{
		header: make(http.Header),
		conn:   conn1,
		rw:     bufio.NewReadWriter(bufio.NewReader(conn1), bufio.NewWriter(conn1)),
	}
}

func (w *testHijackResponseWriter) Header() http.Header {
	return w.header
}

func (w *testHijackResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *testHijackResponseWriter) WriteHeader(int) {}

func (w *testHijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, nil
}
