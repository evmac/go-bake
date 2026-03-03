package env

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkLoadDotenv(b *testing.B) {
	dir := b.TempDir()
	content := "KEY1=value1\nKEY2=value2\n"
	for i := 0; i < 50; i++ {
		_ = os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0644)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadDotenv(dir, []string{".env"})
	}
}

func BenchmarkMerge(b *testing.B) {
	process := os.Environ()
	dotenv := map[string]string{"A": "1", "B": "2"}
	target := map[string]string{"C": "3"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Merge(process, dotenv, target)
	}
}
