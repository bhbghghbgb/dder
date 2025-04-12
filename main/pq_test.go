package main

import (
	"container/heap"
	"testing"
)

// TestUnboundedPriorityQueue tests the priority queue with various input orders.
func TestUnboundedPriorityQueue(t *testing.T) {
	// Helper function to push items into the queue
	pushItems := func(pq *UnboundedPriorityQueue[string], items []Item[string]) {
		for i := range items {
			heap.Push(pq, &items[i])
		}
	}

	// Helper function to validate the output
	validateOrder := func(pq *UnboundedPriorityQueue[string], expectedOrder []Item[string]) {
		for i := range expectedOrder {
			item := heap.Pop(pq).(*Item[string])
			if item.Priority != expectedOrder[i].Priority || item.Value != expectedOrder[i].Value {
				t.Errorf("Expected (Value: %s, Priority: %d), got (Value: %s, Priority: %d)",
					expectedOrder[i].Value, expectedOrder[i].Priority, item.Value, item.Priority)
			}
		}
	}

	// Test Case 1: Ascending priority order
	items1 := []Item[string]{
		{Value: "Item1", Priority: 1},
		{Value: "Item2", Priority: 2},
		{Value: "Item3", Priority: 3},
		{Value: "Item4", Priority: 4},
		{Value: "Item5", Priority: 5},
		{Value: "Item6", Priority: 6},
		{Value: "Item7", Priority: 7},
		{Value: "Item8", Priority: 8},
		{Value: "Item9", Priority: 9},
		{Value: "Item10", Priority: 10},
	}

	expectedOrder1 := []Item[string]{
		{Value: "Item10", Priority: 10},
		{Value: "Item9", Priority: 9},
		{Value: "Item8", Priority: 8},
		{Value: "Item7", Priority: 7},
		{Value: "Item6", Priority: 6},
		{Value: "Item5", Priority: 5},
		{Value: "Item4", Priority: 4},
		{Value: "Item3", Priority: 3},
		{Value: "Item2", Priority: 2},
		{Value: "Item1", Priority: 1},
	}

	pq1 := make(UnboundedPriorityQueue[string], 0)
	pushItems(&pq1, items1)
	validateOrder(&pq1, expectedOrder1)

	// Test Case 2: Descending priority order
	items2 := []Item[string]{
		{Value: "Item10", Priority: 10},
		{Value: "Item9", Priority: 9},
		{Value: "Item8", Priority: 8},
		{Value: "Item7", Priority: 7},
		{Value: "Item6", Priority: 6},
		{Value: "Item5", Priority: 5},
		{Value: "Item4", Priority: 4},
		{Value: "Item3", Priority: 3},
		{Value: "Item2", Priority: 2},
		{Value: "Item1", Priority: 1},
	}

	expectedOrder2 := expectedOrder1 // Same as Test Case 1 since pop order depends on priority

	pq2 := make(UnboundedPriorityQueue[string], 0)
	pushItems(&pq2, items2)
	validateOrder(&pq2, expectedOrder2)

	// Test Case 3: Random priority order
	items3 := []Item[string]{
		{Value: "Item3", Priority: 3},
		{Value: "Item7", Priority: 7},
		{Value: "Item1", Priority: 1},
		{Value: "Item10", Priority: 10},
		{Value: "Item5", Priority: 5},
		{Value: "Item6", Priority: 6},
		{Value: "Item2", Priority: 2},
		{Value: "Item9", Priority: 9},
		{Value: "Item8", Priority: 8},
		{Value: "Item4", Priority: 4},
	}

	expectedOrder3 := expectedOrder1 // Same as Test Case 1 since pop order depends on priority

	pq3 := make(UnboundedPriorityQueue[string], 0)
	pushItems(&pq3, items3)
	validateOrder(&pq3, expectedOrder3)
}
