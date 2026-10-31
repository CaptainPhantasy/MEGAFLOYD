package pqueue

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPriorityQueueOrderingDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	q := New[string]()
	done := make(chan []string, 1)
	go func() {
		q.PushValue("last", 10)
		q.PushValue("first", -1)
		q.PushValue("middle", 3)
		values := []string{}
		for {
			v, ok := q.PopValue()
			if !ok {
				break
			}
			values = append(values, v)
		}
		done <- values
	}()
	select {
	case values := <-done:
		require.Equal(t, []string{"first", "middle", "last"}, values)
	case <-time.After(time.Second):
		t.Fatal("queue blocked while adding or removing an item")
	}
	require.Zero(t, q.Len())
	_, ok := q.Peek()
	require.False(t, ok)
	require.Nil(t, q.PeekItem())
}

func TestPriorityQueueConcurrentProducersAndPurge(t *testing.T) {
	t.Parallel()
	q := New[int]()
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for i := range 32 {
			wg.Add(1)
			go func() { defer wg.Done(); q.PushValue(i, i) }()
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent producers blocked")
	}
	require.Equal(t, 32, q.Len())
	v, ok := q.Peek()
	require.True(t, ok)
	require.Zero(t, v)
	require.Zero(t, q.PeekItem().Priority)
	for want := range 32 {
		v, ok = q.PopValue()
		require.True(t, ok)
		require.Equal(t, want, v)
	}
	q.PushValue(10, 1)
	q.Purge()
	require.Zero(t, q.Len())
	_, ok = q.PopValue()
	require.False(t, ok)
}
