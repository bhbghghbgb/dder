package main

import (
	"sync"
	"sync/atomic"
)

func linterTest() {
	var a int32 = 0

	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			a = atomic.AddInt32(&a, 1)
			wg.Done()
		}()
	}
	wg.Wait()
}

// [{
// 	"resource": "/e:/hocgo/dder/test/linter-test.go",
// 	"owner": "_generated_diagnostic_collection_name_#5",
// 	"code": {
// 		"value": "default",
// 		"target": {
// 			"$mid": 1,
// 			"external": "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/atomic",
// 			"path": "/golang.org/x/tools/go/analysis/passes/atomic",
// 			"scheme": "https",
// 			"authority": "pkg.go.dev"
// 		}
// 	},
// 	"severity": 4,
// 	"message": "direct assignment to atomic value",
// 	"source": "atomic",
// 	"startLineNumber": 15,
// 	"startColumn": 4,
// 	"endLineNumber": 15,
// 	"endColumn": 5
// }]
