// Package pqueue provides a thread-safe priority queue implementation.
package pqueue

import (
	"container/heap"
	"sync"
)

// Item represents an element in the priority queue.
type Item[T any] struct {
	Value    any
	Priority int
	index    int
}

// PriorityQueue is a thread-safe min-heap priority queue.
// Lower priority values are returned first.
type PriorityQueue[T any] struct {
	items []*Item[T]
	mu    sync.RWMutex
}

// New creates a new empty priority queue.
func New[T any]() *PriorityQueue[T] {
	pq := &PriorityQueue[T]{
		items: make([]*Item[T], 0),
	}
	heap.Init(pq)
	return pq
}

// Len returns the number of elements in the queue.
// This method is thread-safe.
func (pq *PriorityQueue[T]) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}

// Less compares two items by priority. Required by heap.Interface.
func (pq *PriorityQueue[T]) Less(i, j int) bool {
	return pq.items[i].Priority < pq.items[j].Priority
}

// Swap exchanges two items. Required by heap.Interface.
func (pq *PriorityQueue[T]) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].index = i
	pq.items[j].index = j
}

// Push adds an item to the queue. Required by heap.Interface.
// Note: This is the internal heap.Push method. Use PushValue for the thread-safe public API.
func (pq *PriorityQueue[T]) Push(x any) {
	n := len(pq.items)
	item := x.(*Item[T])
	item.index = n
	pq.items = append(pq.items, item)
}

// Pop removes and returns the minimum item. Required by heap.Interface.
// Note: This is the internal heap.Pop method. Use PopValue for the thread-safe public API.
func (pq *PriorityQueue[T]) Pop() any {
	old := pq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	pq.items = old[0 : n-1]
	return item
}

// PushValue adds a value with the given priority to the queue.
// This method is thread-safe.
func (pq *PriorityQueue[T]) PushValue(value T, priority int) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	item := &Item[T]{
		Value:    value,
		Priority: priority,
	}
	heap.Push(pq, item)
}

// PopValue removes and returns the highest priority item (lowest priority number).
// Returns the value and true if successful, zero value and false if queue is empty.
// This method is thread-safe.
func (pq *PriorityQueue[T]) PopValue() (T, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		var zero T
		return zero, false
	}

	item := heap.Pop(pq).(*Item[T])
	return item.Value.(T), true
}

// Peek returns the highest priority item without removing it.
// Returns the value and true if successful, zero value and false if queue is empty.
// This method is thread-safe.
func (pq *PriorityQueue[T]) Peek() (T, bool) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if len(pq.items) == 0 {
		var zero T
		return zero, false
	}

	return pq.items[0].Value.(T), true
}

// Purge removes all items from the queue.
// This method is thread-safe.
func (pq *PriorityQueue[T]) Purge() {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Clear items and help GC
	for i := range pq.items {
		pq.items[i] = nil
	}
	pq.items = pq.items[:0]
}

// PeekItem returns the highest priority item (including priority value) without removing it.
// Returns nil if queue is empty.
func (pq *PriorityQueue[T]) PeekItem() *Item[T] {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if len(pq.items) == 0 {
		return nil
	}
	return pq.items[0]
}
