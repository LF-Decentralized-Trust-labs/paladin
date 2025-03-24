package controller

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInFlight(t *testing.T) {
	inflight := NewInFlight(30 * time.Second)
	require.NotNil(t, inflight)
	require.NotNil(t, inflight.mux)
	require.NotNil(t, inflight.inFlight)
	assert.Equal(t, 30*time.Second, inflight.changeWaitDuration)
}

func TestInFlight_Insert(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "testKey"

	// Insert a new key
	inserted := inflight.Insert(key)
	assert.True(t, inserted, "Expected Insert to return true for new key")

	// Try inserting the same key again
	inserted = inflight.Insert(key)
	assert.False(t, inserted, "Expected Insert to return false for existing key")
}

func TestInFlight_IsQueued(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "testKey"

	// Key should not be queued initially
	isQueued := inflight.IsQueued(key)
	assert.False(t, isQueued, "Expected IsQueued to return false for non-existent key")

	// Insert the key
	inflight.Insert(key)

	// Key should now be queued
	isQueued = inflight.IsQueued(key)
	assert.True(t, isQueued, "Expected IsQueued to return true for existing key")
}

func TestInFlight_IsReady(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "testKey"
	inflight.changeWaitDuration = 1 * time.Second

	// IsReady should return false for non-existent key
	isReady := inflight.IsReady(key)
	assert.False(t, isReady, "Expected IsReady to return false for non-existent key")

	// Insert the key
	inflight.Insert(key)

	// IsReady should return false immediately after insertion
	isReady = inflight.IsReady(key)
	assert.False(t, isReady, "Expected IsReady to return false immediately after insertion")

	// Wait for less than changeWaitDuration
	time.Sleep(inflight.changeWaitDuration / 2)

	// IsReady should still return false
	isReady = inflight.IsReady(key)
	assert.False(t, isReady, "Expected IsReady to return false before changeWaitDuration")

	// Wait until changeWaitDuration has passed
	time.Sleep(inflight.changeWaitDuration / 2)

	// IsReady should now return true
	isReady = inflight.IsReady(key)
	assert.True(t, isReady, "Expected IsReady to return true after changeWaitDuration")
}

func TestInFlight_Delete(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "testKey"

	// Insert the key
	inflight.Insert(key)

	// Delete the key
	inflight.Delete(key)

	// IsQueued should return false
	isQueued := inflight.IsQueued(key)
	assert.False(t, isQueued, "Expected IsQueued to return false after deletion")

	// IsReady should return false
	isReady := inflight.IsReady(key)
	assert.False(t, isReady, "Expected IsReady to return false after deletion")
}

func TestInFlight_Delete_NonExistentKey(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "nonExistentKey"

	// Delete a non-existent key (should not cause panic)
	inflight.Delete(key)

	// IsQueued should return false
	isQueued := inflight.IsQueued(key)
	assert.False(t, isQueued, "Expected IsQueued to return false for non-existent key")
}

func TestInFlight_ConcurrentAccess(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "testKey"
	var wg sync.WaitGroup

	// Test concurrent Insert
	wg.Add(2)
	go func() {
		defer wg.Done()
		inflight.Insert(key)
	}()
	go func() {
		defer wg.Done()
		inflight.Insert(key)
	}()
	wg.Wait()

	// IsQueued should return true
	isQueued := inflight.IsQueued(key)
	assert.True(t, isQueued, "Expected IsQueued to return true after concurrent Insert")

	// Test concurrent IsReady
	wg.Add(2)
	go func() {
		defer wg.Done()
		inflight.IsReady(key)
	}()
	go func() {
		defer wg.Done()
		inflight.IsReady(key)
	}()
	wg.Wait()

	// Test concurrent Delete
	wg.Add(2)
	go func() {
		defer wg.Done()
		inflight.Delete(key)
	}()
	go func() {
		defer wg.Done()
		inflight.Delete(key)
	}()
	wg.Wait()

	// IsQueued should return false
	isQueued = inflight.IsQueued(key)
	assert.False(t, isQueued, "Expected IsQueued to return false after concurrent Delete")
}

func TestInFlight_InsertMultipleKeys(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	keys := []string{"key1", "key2", "key3"}
	inflight.changeWaitDuration = 1 * time.Second

	// Insert multiple keys
	for _, key := range keys {
		inserted := inflight.Insert(key)
		assert.True(t, inserted, "Expected Insert to return true for new key: %s", key)
	}

	// Verify all keys are queued
	for _, key := range keys {
		isQueued := inflight.IsQueued(key)
		assert.True(t, isQueued, "Expected IsQueued to return true for key: %s", key)
	}

	// Wait for changeWaitDuration
	time.Sleep(inflight.changeWaitDuration)

	// Verify all keys are ready
	for _, key := range keys {
		isReady := inflight.IsReady(key)
		assert.True(t, isReady, "Expected IsReady to return true for key: %s", key)
	}
}

func TestInFlight_IsReady_NoEntries(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "nonExistentKey"

	// IsReady should return false for a key that was never inserted
	isReady := inflight.IsReady(key)
	assert.False(t, isReady, "Expected IsReady to return false for non-existent key")
}

func TestInFlight_IsQueued_NoEntries(t *testing.T) {
	inflight := NewInFlight(1 * time.Second)
	key := "nonExistentKey"

	// IsQueued should return false for a key that was never inserted
	isQueued := inflight.IsQueued(key)
	assert.False(t, isQueued, "Expected IsQueued to return false for non-existent key")
}
