package main

import (
	"container/heap"
	"sync"
)

// https://github.com/adrianbrad/queue/blob/main/blocking.go
// https://github.com/theodesp/blockingQueues/blob/master/blockingQueue.go
// https://pkg.go.dev/container/heap#example-package-PriorityQueue

// Item represents a generic item with a priority.
type Item[T any] struct {
	Value    T
	Priority int // Higher value means higher priority
	index    int // Index in the heap (for heap.Interface)
}

// UnboundedPriorityQueue implements heap.Interface and holds Items.
type UnboundedPriorityQueue[T any] []*Item[T]

func (pq UnboundedPriorityQueue[T]) Len() int { return len(pq) }

// We want higher priority to have a lower index in the heap for removal.
func (pq UnboundedPriorityQueue[T]) Less(i, j int) bool {
	return pq[i].Priority > pq[j].Priority
}

func (pq UnboundedPriorityQueue[T]) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *UnboundedPriorityQueue[T]) Push(x any) {
	n := len(*pq)
	item := x.(*Item[T])
	item.index = n
	*pq = append(*pq, item)
}

func (pq *UnboundedPriorityQueue[T]) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leaks
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// update modifies the priority of an Item in the queue.
func (pq *UnboundedPriorityQueue[T]) update(item *Item[T], value T, priority int) {
	item.Value = value
	item.Priority = priority
	heap.Fix(pq, item.index)
}

// BlockingPriorityQueue wraps UnboundedPriorityQueue with thread safety.
type BlockingPriorityQueue[T any] struct {
	pq UnboundedPriorityQueue[T] // The underlying priority queue
	mu sync.Mutex                // Mutex for thread-safe access to the queue
	co *sync.Cond                // Condition variable for signaling when the queue is not empty
}

// NewBlockingPriorityQueue initializes a new BlockingPriorityQueue.
func NewBlockingPriorityQueue[T any]() *BlockingPriorityQueue[T] {
	bpq := &BlockingPriorityQueue[T]{}
	// Initialize the condition variable with the mutex
	bpq.co = sync.NewCond(&bpq.mu)
	return bpq
}

// Len returns the length of the priority queue in a thread-safe manner.
func (pqw *BlockingPriorityQueue[T]) Len() int {
	pqw.mu.Lock()         // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock() // Release the lock when the function exits
	return pqw.pq.Len()
}

// Push adds an item to the priority queue in a thread-safe manner.
func (pqw *BlockingPriorityQueue[T]) Push(x *Item[T]) {
	pqw.mu.Lock()         // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock() // Release the lock when the function exits
	defer pqw.co.Signal() // Signal one waiting goroutine that an item has been added
	heap.Push(&pqw.pq, x) // Add the item to the underlying priority queue
}

// Pop removes and returns the highest-priority item from the queue in a thread-safe manner.
func (pqw *BlockingPriorityQueue[T]) Pop() *Item[T] {
	pqw.mu.Lock() // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock()

	// Wait until the queue is not empty
	for pqw.pq.Len() <= 0 {
		pqw.co.Wait() // Release the lock and wait for a signal
	}

	// Remove and return the highest-priority item
	return heap.Pop(&pqw.pq).(*Item[T])
}
