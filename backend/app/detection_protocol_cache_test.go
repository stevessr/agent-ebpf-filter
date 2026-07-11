package app

import (
	"fmt"
	"testing"
	"time"
)

func TestProtoDetectionCacheLookupDeletesExpiredEntry(t *testing.T) {
	cache := newProtoDetectionCache()
	cache.entries["expired"] = &protoDetectionEntry{
		DetectedAt: time.Now().UTC().Add(-protoDetectionTTL - time.Second),
	}

	if _, ok := cache.Lookup("expired"); ok {
		t.Fatal("expired protocol cache entry was returned")
	}
	if _, ok := cache.entries["expired"]; ok {
		t.Fatal("expired protocol cache entry was not deleted")
	}
}

func TestProtoDetectionCacheBoundsHighCardinalityDestinations(t *testing.T) {
	cache := newProtoDetectionCache()
	for i := 0; i <= protoDetectionMaxEntries; i++ {
		cache.Record(fmt.Sprintf("destination-%d", i), AppProtoTLS, "", "", "", "")
	}

	if got := len(cache.entries); got > protoDetectionMaxEntries {
		t.Fatalf("protocol cache entries = %d, max = %d", got, protoDetectionMaxEntries)
	}
	if _, ok := cache.Lookup(fmt.Sprintf("destination-%d", protoDetectionMaxEntries)); !ok {
		t.Fatal("newest protocol cache entry was evicted")
	}
}

func TestProtoDetectionCacheRefreshAtCapacityDoesNotEvictPeer(t *testing.T) {
	cache := newProtoDetectionCache()
	for i := 0; i < protoDetectionMaxEntries; i++ {
		cache.Record(fmt.Sprintf("destination-%d", i), AppProtoTLS, "", "", "", "")
	}

	cache.Record("destination-1", AppProtoHTTP, "", "", "", "GET")
	if got := len(cache.entries); got != protoDetectionMaxEntries {
		t.Fatalf("protocol cache entries = %d, want %d", got, protoDetectionMaxEntries)
	}
	if _, ok := cache.Lookup("destination-0"); !ok {
		t.Fatal("refreshing an existing cache key evicted another destination")
	}
}
