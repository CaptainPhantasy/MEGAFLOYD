package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBrokerFanoutBackpressureAndCancellation(t *testing.T) {
	t.Parallel()
	b := NewBrokerWithOptions[string](1)
	defer b.Shutdown()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := b.Subscribe(ctx)
	otherCtx, otherCancel := context.WithCancel(context.Background())
	defer otherCancel()
	other := b.Subscribe(otherCtx)
	b.Publish(CreatedEvent, "first")
	require.Equal(t, Event[string]{Type: CreatedEvent, Payload: "first"}, <-a)
	require.Equal(t, "first", (<-other).Payload)
	b.Publish(UpdatedEvent, "buffered")
	b.Publish(UpdatedEvent, "dropped")
	require.Equal(t, uint64(2), b.DropCount())
	b.SetMustDeliverTimeout(time.Millisecond)
	b.PublishMustDeliver(ctx, DeletedEvent, "terminal")
	require.Equal(t, uint64(2), b.MustDeliverDropCount())
	require.Equal(t, "buffered", (<-a).Payload)
	require.Equal(t, "buffered", (<-other).Payload)
	b.PublishMustDeliver(ctx, DeletedEvent, "done")
	require.Equal(t, "done", (<-a).Payload)
	require.Equal(t, "done", (<-other).Payload)
	cancel()
	select {
	case _, ok := <-a:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("cancelled subscriber stayed open")
	}
	require.Equal(t, 1, b.GetSubscriberCount())
	b.Shutdown()
	_, ok := <-other
	require.False(t, ok)
	_, ok = <-b.Subscribe(context.Background())
	require.False(t, ok)
}

func TestBrokerConcurrentShutdown(t *testing.T) {
	t.Parallel()
	for range 100 {
		b := NewBroker[int]()
		ready := make(chan struct{})
		panics := make(chan any, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if p := recover(); p != nil {
						panics <- p
					}
				}()
				<-ready
				b.Shutdown()
			}()
		}
		close(ready)
		wg.Wait()
		close(panics)
		for p := range panics {
			t.Errorf("shutdown panicked: %v", p)
		}
	}
}
