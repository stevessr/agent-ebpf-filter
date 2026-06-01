package main

import (
	"testing"
	"time"
)

func TestTLSCaptureStoreKeepsOnlyMostRecentEvents(t *testing.T) {
	store := NewTLSCaptureStore(2)
	store.Add(TLSPlaintextEvent{PID: 1, Timestamp: time.Unix(1, 0)})
	store.Add(TLSPlaintextEvent{PID: 2, Timestamp: time.Unix(2, 0)})
	store.Add(TLSPlaintextEvent{PID: 3, Timestamp: time.Unix(3, 0)})

	recent := store.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("recent len = %d, want 2", len(recent))
	}
	if recent[0].PID != 2 || recent[1].PID != 3 {
		t.Fatalf("recent = %#v, want PIDs 2 and 3", recent)
	}
}

func TestTLSCaptureStoreTracksLibraryStatuses(t *testing.T) {
	store := NewTLSCaptureStore(10)
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/usr/lib/libssl.so", Attached: true, Available: true})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so", Error: "missing symbol"})

	statuses := store.LibraryStatuses()
	if len(statuses) != 2 {
		t.Fatalf("statuses len = %d, want 2", len(statuses))
	}

	var seenOpenSSL, seenGnuTLS bool
	for _, status := range statuses {
		switch status.Name {
		case "OpenSSL":
			seenOpenSSL = status.Attached && status.Available && status.Path == "/usr/lib/libssl.so"
		case "GnuTLS":
			seenGnuTLS = status.Error == "missing symbol" && status.Path == "/usr/lib/libgnutls.so"
		}
	}
	if !seenOpenSSL || !seenGnuTLS {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestTLSCaptureStoreLibraryStatusesAreSortedByNameThenPath(t *testing.T) {
	store := NewTLSCaptureStore(10)
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/opt/libssl.so"})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so"})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/usr/lib/libssl.so"})

	statuses := store.LibraryStatuses()
	if len(statuses) != 3 {
		t.Fatalf("statuses len = %d, want 3", len(statuses))
	}

	want := []TLSLibraryStatus{
		{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so"},
		{Name: "OpenSSL", Path: "/opt/libssl.so"},
		{Name: "OpenSSL", Path: "/usr/lib/libssl.so"},
	}
	for i, status := range statuses {
		if status.Name != want[i].Name || status.Path != want[i].Path {
			t.Fatalf("status[%d] = %#v, want %#v", i, status, want[i])
		}
	}
}
