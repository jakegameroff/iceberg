package main

import (
	"sync"
	"time"
)

// ttl is how long a topic lives before it's swept.
const ttl = 10 * time.Minute

type entry struct {
	body    []byte
	expires time.Time
}

var mu sync.RWMutex
var store = make(map[string]entry)

func readStore(k string) []byte {
	mu.RLock()
	defer mu.RUnlock()

	e, ok := store[k]
	if !ok || time.Now().After(e.expires) {
		return nil
	}

	return e.body
}

func writeStore(k string, v []byte) {
	mu.Lock()
	defer mu.Unlock()

	store[k] = entry{
		body:    v,
		expires: time.Now().Add(ttl),
	}
}

func cleanStore() {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	for k, e := range store {
		if now.After(e.expires) {
			delete(store, k)
		}
	}
}

func startCleaner(every time.Duration) {
	go func() {
		for {
			time.Sleep(every)
			cleanStore()
		}
	}()
}
