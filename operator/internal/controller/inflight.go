package controller

import (
	"sync"
	"time"
)

// InFlight is a struct used to manage in flight requests for a unique identifier.
type InFlight struct {
	mux                *sync.RWMutex
	inFlight           map[string]time.Time
	changeWaitDuration time.Duration
}

// NewInFlight instanciates a InFlight structures.
func NewInFlight(changeWaitDuration time.Duration) *InFlight {
	return &InFlight{
		mux:                &sync.RWMutex{},
		inFlight:           make(map[string]time.Time),
		changeWaitDuration: changeWaitDuration,
	}
}

func (db *InFlight) Duration() time.Duration {
	return db.changeWaitDuration
}

func (db *InFlight) IsQueued(key string) bool {
	db.mux.RLock()
	defer db.mux.RUnlock()

	_, ok := db.inFlight[key]
	return ok
}

func (db *InFlight) IsReady(key string) bool {
	db.mux.RLock()
	defer db.mux.RUnlock()

	t, ok := db.inFlight[key]
	if !ok {
		return false
	}

	return time.Since(t) >= db.changeWaitDuration
}

// Insert inserts the entry to the current list of inflight, request key is a unique identifier.
// Returns false when the key already exists.
func (db *InFlight) Insert(key string) bool {
	db.mux.Lock()
	defer db.mux.Unlock()

	_, ok := db.inFlight[key]
	if ok {
		return false
	}

	db.inFlight[key] = time.Now().UTC()
	return true
}

// Delete removes the entry from the inFlight entries map.
// It doesn't return anything, and will do nothing if the specified key doesn't exist.
func (db *InFlight) Delete(key string) {
	db.mux.Lock()
	defer db.mux.Unlock()

	delete(db.inFlight, key)
}
