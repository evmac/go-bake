package dsl

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkParseMinimal(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "minimal.bake")
	src, err := readFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Parser.ParseString(path, src)
	}
}

func BenchmarkParseAndCompileMinimal(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "minimal.bake")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseAndCompile(path)
	}
}

func readFile(path string) (string, error) {
	b, err := readFileBytes(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
