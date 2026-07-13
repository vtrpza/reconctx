package main

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCompileFixture(b *testing.B) {
	fixture := filepath.Join("..", "..", "fixtures", "cases", "katana", "v1.6.1", "KAT-NORMAL-MINIMAL", "native-output.jsonl")
	root := b.TempDir()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := filepath.Join(root, "run")
		if _, err := CompileFixture(fixture, out); err != nil {
			b.Fatal(err)
		}
		if err := os.RemoveAll(out); err != nil {
			b.Fatal(err)
		}
	}
}
